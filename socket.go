package main

/*
 Frame:
 - https://datatracker.ietf.org/doc/html/rfc6455#section-5.2
 - https://developer.mozilla.org/zh-CN/docs/Web/API/WebSockets_API/Writing_WebSocket_servers

      0                   1                   2                   3
      0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
     +-+-+-+-+-------+-+-------------+-------------------------------+
     |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
     |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
     |N|V|V|V|       |S|             |   (if payload len==126/127)   |
     | |1|2|3|       |K|             |                               |
     +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
     |     Extended payload length continued, if payload len == 127  |
     + - - - - - - - - - - - - - - - +-------------------------------+
     |                               |Masking-key, if MASK set to 1  |
     +-------------------------------+-------------------------------+
     | Masking-key (continued)       |          Payload Data         |
     +-------------------------------- - - - - - - - - - - - - - - - +
     :                     Payload Data continued ...                :
     + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
     |                     Payload Data continued ...                |
     +---------------------------------------------------------------+

 操作码 (opcode)
   -  %x0 denotes a continuation frame
   -  %x1 denotes a text frame
   -  %x2 denotes a binary frame
   -  %x3-7 are reserved for further non-control frames
   -  %x8 denotes a connection close
   -  %x9 denotes a ping
   -  %xA denotes a pong
   -  %xB-F are reserved for further control frames

*/

type Socket struct {
	client                *WebSocketClient
	closed                bool
	bufferedBytes         uint64   // 所有缓存 buffer 的字节数
	buffers               []Buffer // 所有缓存的buffer
	fragments             []Buffer // 保存分片内容
	isContinueReceiveData bool

	fin                bool       // 分片是否结束（true表示结束）
	opcode             Opcode     // 操作码
	masked             bool       // 是否使用掩码
	maskingKey         MaskingKey // 掩码key
	payloadLength      uint64     // 当前帧数据载荷长度
	totalPayloadLength uint64     // 数据载荷总长度
}

func NewSocket(client *WebSocketClient) *Socket {
	socket := Socket{
		client:     client,
		buffers:    make([]Buffer, 0, 10),
		fragments:  make([]Buffer, 0, 10),
		maskingKey: MaskingKey{},
	}
	return &socket
}

func (s *Socket) append(chunk Buffer) {
	if s.opcode == 0x08 { // 已断开
		return
	}

	s.bufferedBytes += uint64(len(chunk))
	s.buffers = append(s.buffers, chunk)

	if !s.isContinueReceiveData {
		s.receiveHeader()
	}
	s.receiveData()
}

func (s *Socket) receiveHeader() {
	if s.bufferedBytes < 2 {
		return
	}

	buf := s.consume(2)
	s.fin = (buf[0] & 0b10000000) == 0b10000000
	opcode := Opcode(buf[0] & 0b00001111)
	if opcode != 0x00 { // 如果是分片传输，使用之前的 opcode
		s.opcode = opcode
	}
	s.masked = (buf[1] & 0b10000000) == 0b10000000

	s.payloadLength = uint64(buf[1] & 0b01111111)
	if s.payloadLength == 126 { // 16 bits
		buf = s.consume(2)
		s.payloadLength = uint64(buf[0]) << 8
		s.payloadLength += uint64(buf[1])
	} else if s.payloadLength == 127 { // 64 bits
		buf = s.consume(8)
		for i := 0; i < 8; i++ {
			s.payloadLength += uint64(buf[i]) << (8 * (8 - i - 1))
		}
	}
	s.totalPayloadLength += s.payloadLength // 记录分片传输场景的收到总字节数

	if s.masked {
		buf = s.consume(4)
		copy(s.maskingKey[:], buf)
	}
}

func (s *Socket) receiveData() {
	data := make(Buffer, 0)
	if s.payloadLength > 0 {
		if s.bufferedBytes < s.totalPayloadLength { // 等待缓冲区分段读取完成
			s.isContinueReceiveData = true
			return
		} else {
			s.isContinueReceiveData = false
		}

		data = s.consume(int(s.payloadLength))
		if s.masked {
			Mask(data, s.maskingKey)
		}
	}

	// control frames
	if s.opcode >= 0x08 {
		s.handleControlFrame(data)
		s.totalPayloadLength = 0
		s.fragments = s.fragments[:0]
		return
	}

	s.fragments = append(s.fragments, data)

	if s.fin { // 分片传输结束
		message := make(Buffer, 0, s.totalPayloadLength)
		for _, fragment := range s.fragments {
			message = append(message, fragment...)
		}
		isBinary := s.opcode == 0x02 // 二进制数据
		s.client.Emit(EventMessage, nil, message, isBinary)

		s.totalPayloadLength = 0
		s.fragments = s.fragments[:0]
	}
}

func (s *Socket) handleControlFrame(data Buffer) {
	if s.opcode == 0x08 { // disconnect
		code := int(data[0]) << 8
		code += int(data[1])
		reason := data[:2]
		s.client.Emit(EventClose, nil, reason, false)
		return
	}

	if s.opcode == 0x09 { // ping
		s.client.Emit(EventPing, nil, data, false)
		s.pong(data)
	} else if s.opcode == 0x0a { // pong
		s.client.Emit(EventPong, nil, data, false)
	}
}

func (s *Socket) consume(n int) Buffer {
	s.bufferedBytes -= uint64(n)
	buf := make(Buffer, 0, n)
	bytes := n
	consumeCount := 0

	for index, buffer := range s.buffers {
		if bytes <= 0 {
			break
		}
		if len(buffer) > bytes {
			s.buffers[index] = buffer[bytes:]
			buffer = buffer[:bytes]
		} else {
			consumeCount += 1
		}

		bytes -= len(buffer)
		buf = append(buf, buffer...)
	}

	if consumeCount > 0 {
		s.buffers = s.buffers[consumeCount:]
	}

	return buf[:n]
}

func (s *Socket) ping(data Buffer) {
	buf := BuildFrame(data, 0x09, true, true)
	s.client.conn.Write(buf)
}

func (s *Socket) pong(data Buffer) {
	buf := BuildFrame(data, 0x0a, true, true)
	s.client.conn.Write(buf)
}

package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"github.com/gobwas/httphead"
	"math/rand"
	"net/http"
	"net/textproto"
	"strings"
)

type (
	Event      uint8
	Opcode     uint8
	MaskingKey [4]byte
	Buffer     []byte
)

type Header struct {
	method     string
	version    string
	statusCode int
	message    string
	headers    map[string]string
}

const MagicGuid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
const (
	EventMessage Event = iota
	EventClose
	EventError
	EventPing
	EventPong
)

func ParseResponseHeader(buffer Buffer) *Header {
	header := Header{
		headers: make(map[string]string, 10),
	}

	response := string(buffer[:])
	index := strings.Index(response, "\r\n")
	if index > 0 {
		head, ok := httphead.ParseResponseLine(buffer[0:index])
		if ok {
			header.version = fmt.Sprintf("%d.%d", head.Version.Major, head.Version.Minor)
			header.statusCode = head.Status
			header.message = string(head.Reason)
		}

		reader := bufio.NewReader(strings.NewReader(string(buffer[index+2:])))
		tp := textproto.NewReader(reader)
		mimeHeader, err := tp.ReadMIMEHeader()
		if err == nil {
			headers := http.Header(mimeHeader)
			for key, value := range headers {
				header.headers[strings.ToLower(key)] = strings.Join(value, "")
			}
		}

	}
	return &header
}

func HashKey(key *string) string {
	sha1Bytes := sha1.Sum(Buffer(*key + MagicGuid))
	return base64.StdEncoding.EncodeToString(sha1Bytes[:])
}

func Mask(buffer Buffer, maskingKey MaskingKey) {
	for i := 0; i < len(buffer); i += 1 {
		buffer[i] = buffer[i] ^ maskingKey[i%4]
	}
}

func BuildFrame(data Buffer, opcode Opcode, masked bool, fin bool) Buffer {
	length := len(data)
	payloadLength := length
	offset := 2 // 保留头部信息字节数
	if masked {
		offset += 4
	}

	if length > 0b11111111_11111111 {
		// 长度大于`16位最大整数`，使用64位无符号整数
		payloadLength = 127
		offset += 8
	} else if length > 125 {
		// 长度大于125, 使用16位无符号整数
		payloadLength = 126
		offset += 2
	}

	buffer := make(Buffer, offset, offset+length)

	// 写入第一个8位
	buffer[0] = byte(opcode)
	if fin {
		buffer[0] = 0b10000000 | buffer[0]
	}
	// 写入第二个8位
	buffer[1] = byte(payloadLength)
	if masked {
		buffer[1] = 0b10000000 | buffer[1]

	}

	if payloadLength == 126 {
		buffer[2] = byte(length >> 8)
		buffer[3] = byte(length - int(buffer[2]))
	} else if payloadLength == 127 {
		remain := length
		for i := 0; i < 8; i++ {
			buffer[2+i] = byte(remain - (length >> (8 * (8 - i - 1))))
		}
	}

	// 写入 maskingKey
	if masked {
		maskingKey := MaskingKey{
			byte(rand.Intn(255)),
			byte(rand.Intn(255)),
			byte(rand.Intn(255)),
			byte(rand.Intn(255)),
		}
		copy(buffer[offset-4:offset], maskingKey[:])
		Mask(data, maskingKey)
		buffer = append(buffer, data...)
	} else {
		buffer = append(buffer, data...)
	}

	return buffer
}

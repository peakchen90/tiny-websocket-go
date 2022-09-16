package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"sync"
)

type EventHandler = func(err error, data Buffer, binary bool)

type WebSocketClient struct {
	conn        net.Conn
	socket      *Socket
	listeners   map[Event][]*EventHandler
	isConnected bool
	isClosed    bool
	waitGroup   sync.WaitGroup
}

func WebsocketConnect(rawURL string) (*WebSocketClient, error) {
	client := WebSocketClient{
		listeners: make(map[Event][]*EventHandler, 10),
		waitGroup: sync.WaitGroup{},
	}

	URL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial("tcp", URL.Host)
	if err != nil {
		return nil, err
	}

	randomKey := fmt.Sprintf("WebSocket@%.10f", rand.Float64())
	secKey := base64.StdEncoding.EncodeToString(Buffer(randomKey[:]))
	handshake := [...]string{
		fmt.Sprintf("GET %s HTTP/1.1", URL.String()),
		fmt.Sprintf("Host: %s", URL.Hostname()),
		"Upgrade: websocket",
		"Connection: Upgrade",
		fmt.Sprintf("Sec-WebSocket-Key: %s", secKey),
		"Sec-WebSocket-Version: 13",
		"\r\n",
	}
	_, err = conn.Write([]byte(strings.Join(handshake[:], "\r\n")))
	if err != nil {
		return nil, err
	}

	buf := make(Buffer, 1024)
	size, err := conn.Read(buf[:])
	if err != nil {
		return nil, err
	}

	// 解析 response header
	header := ParseResponseHeader(buf[:size])
	headers := header.headers

	if header.statusCode == 101 &&
		strings.ToLower(headers["upgrade"]) == "websocket" &&
		strings.ToLower(headers["connection"]) == "upgrade" &&
		headers["sec-websocket-accept"] == HashKey(&secKey) {
		client.isConnected = true
		client.conn = conn
		client.socket = NewSocket(&client)
		client.waitGroup.Add(1)
		go client.polling()
	} else {
		return nil, errors.New("abort handshake")
	}

	return &client, nil
}

func (w *WebSocketClient) On(event Event, callback EventHandler) {
	target := w.listeners[event]
	if target == nil {
		target = make([]*EventHandler, 0, 10)
	}
	w.listeners[event] = append(target, &callback)
}

func (w *WebSocketClient) Emit(event Event, err error, data Buffer, binary bool) {
	target := w.listeners[event]
	if target != nil {
		for _, handler := range target {
			(*handler)(err, data, binary)
		}
	}
}

func (w *WebSocketClient) Send(binary bool, data ...Buffer) {
	opcode := Opcode(0x01)
	if binary {
		opcode = 0x02
	}
	isFragments := len(data) > 1

	for i := 0; i < len(data); i++ {
		item := data[i]
		_opcode := opcode
		if isFragments && i > 0 {
			_opcode = 0x00
		}
		buf := BuildFrame(item, _opcode, true, i == len(data)-1)
		w.conn.Write(buf)
	}

}

func (w *WebSocketClient) Close(reason Buffer) {
	w.conn.Close()
	w.Emit(EventClose, nil, reason, false)
	w.isConnected = false
	w.isClosed = true
	w.waitGroup.Done()
}

func (w *WebSocketClient) Hold() (unHold func()) {
	w.waitGroup.Wait()
	return func() {
		w.waitGroup.Done()
	}
}

func (w *WebSocketClient) polling() {
	buf := make(Buffer, 65335-4) // 16 bits length (约 64kb)

	for {
		if w.isClosed {
			break
		}

		size, err := w.conn.Read(buf)
		if err != nil {
			w.Emit(EventError, err, nil, false)
		} else {
			w.socket.append(buf[:size])
		}
	}
}

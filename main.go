package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bxcodec/faker/v3"
	"github.com/shabbyrobe/termimg"
	"image/png"
)

type Message struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	Username string `json:"username"`
}

func receive(msg <-chan string) {
	for {
		select {
		case data := <-msg:
			fmt.Println(data)
		}
	}
}

func main() {
	nick := faker.ChineseName()
	client, err := WebsocketConnect("ws://127.0.0.1:3333")
	if err != nil {
		panic(err)
	}

	joinMsg, _ := json.Marshal(Message{"join", nick, nick})
	client.Send(false, joinMsg)

	message := make(chan string)
	go receive(message)

	// listen message
	client.On(EventMessage, func(err error, data Buffer, binary bool) {
		if err != nil {
			return
		}

		if binary {
			nickLength := int(data[0])
			senderNick := string(data[1 : nickLength+1])
			binaryData := data[nickLength+1:]

			img, err := png.Decode(bytes.NewReader(binaryData))
			if err != nil {
				fmt.Println("Error: ", err)
				return
			}
			data := termimg.EscapeData{}
			render, _ := termimg.PresetBitmapBlock().Renderer()
			err = render.Escapes(&data, img, 0)
			if err != nil {
				fmt.Println("Error: ", err)
				return
			}
			message <- fmt.Sprintf("%s: \n", senderNick)
			message <- string(data.Value())
			message <- string("\033[0m") // 恢复 ANSI 默认颜色
		} else {
			msg := Message{}
			err := json.Unmarshal(data, &msg)
			if err != nil {
				return
			}

			switch msg.Type {
			case "join":
				message <- fmt.Sprintf("%s 已加入", msg.Data)
			case "leave":
				message <- fmt.Sprintf("%s 已离开", msg.Data)
			case "message":
				message <- fmt.Sprintf("%s: %s", msg.Username, msg.Data)
			}
		}
	})

	client.Hold()
}

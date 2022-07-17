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

func receive(message string) {
	fmt.Println(message)
}

func main() {
	nick := faker.ChineseName()
	client, err := WebsocketConnect("ws://127.0.0.1:3333")
	if err != nil {
		panic(err)
	}

	joinMsg, _ := json.Marshal(Message{"join", nick, nick})
	client.Send(false, joinMsg)

	// listen message
	client.On(EventMessage, func(err error, data Buffer, binary bool) {
		if err != nil {
			return
		}

		if binary {
			nickLength := int(data[0])
			senderNick := string(data[1 : nickLength+1])
			binaryData := data[nickLength+1:]

			img, _ := png.Decode(bytes.NewReader(binaryData))
			data := termimg.EscapeData{}
			render, _ := termimg.PresetBitmapBlock().Renderer()
			render.Escapes(&data, img, 0)
			receive(fmt.Sprintf("%s: \n%s", senderNick, data.Value()))
		} else {
			msg := Message{}
			err := json.Unmarshal(data, &msg)
			if err != nil {
				return
			}

			switch msg.Type {
			case "join":
				receive(fmt.Sprintf("%s 已加入", msg.Data))
			case "leave":
				receive(fmt.Sprintf("%s 已离开", msg.Data))
			case "message":
				receive(fmt.Sprintf("%s: %s", msg.Username, msg.Data))
			}
		}
	})

	client.Hold()
}

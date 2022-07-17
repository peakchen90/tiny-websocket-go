package main

import (
	"encoding/json"
	"fmt"
	"github.com/bxcodec/faker/v3"
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
			//receive(fmt.Sprintf("%s: %s", msg.Username, msg.Data))
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

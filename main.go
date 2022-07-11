package main

func main() {
	client := WebSocketClient{}
	client.Connect("ws://127.0.0.1:3333/32?3AA#CC")

}

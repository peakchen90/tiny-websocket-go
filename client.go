package main

import (
	"net/url"
)

type WebSocketClient struct {
	URL *url.URL
}

func (client *WebSocketClient) Connect(rawURL string) *WebSocketClient {
	URL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		panic(err)
	}

	client.URL = URL
	return client
}

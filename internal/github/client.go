package github

import (
	"net/http"
	"time"
)

type Client struct {
	Username string
	Token    string
	HTTP     *http.Client
	sem      chan struct{}
}

func NewClient(username, token string, maxConnections int) *Client {
	return &Client{
		Username: username,
		Token:    token,
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
		sem: make(chan struct{}, maxConnections),
	}
}

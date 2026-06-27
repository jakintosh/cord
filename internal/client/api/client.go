package api

import (
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type Client struct {
	t *daemon.Transport
}

func NewClient(
	socketPath string,
) *Client {
	return &Client{
		t: daemon.NewTransport(socketPath),
	}
}

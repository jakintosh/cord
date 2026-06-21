package clientd

import (
	"context"

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

func (c *Client) Status(
	ctx context.Context,
) error {
	resp, err := c.t.Get(ctx, "/status")
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[StatusResponse](resp)
	return err
}

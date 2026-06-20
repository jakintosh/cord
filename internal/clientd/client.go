package clientd

import (
	"context"
	"fmt"
	"net/http"

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
		return fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: unexpected status %s", resp.Status)
	}
	return nil
}

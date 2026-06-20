package control

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

type Client struct {
	socketPath string
	http       *http.Client
}

func NewClient(
	socketPath string,
) *Client {
	return &Client{
		socketPath: socketPath,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

func (c *Client) Ping(
	ctx context.Context,
) error {
	resp, err := c.http.Get("http://unix/ping")
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: unexpected status %s", resp.Status)
	}
	return nil
}

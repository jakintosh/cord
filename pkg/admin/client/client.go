package client

import (
	"errors"
	"net/url"
	"path/filepath"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

// DefaultSocketPath is the default client-daemon administration socket.
const DefaultSocketPath = "/var/run/cord/client.sock"

// Client calls the Cord client daemon's administration API.
type Client struct {
	wire wire.Client
}

// New returns a client that connects to socketPath.
func New(
	socketPath string,
) (
	*Client,
	error,
) {
	if socketPath == "" {
		return nil, errors.New("client admin: socket path required")
	}
	socketPath, err := filepath.Abs(socketPath)
	if err != nil {
		return nil, err
	}
	w, err := wire.NewClient("unix:///"+socketPath, wire.ClientOptions{})
	if err != nil {
		return nil, err
	}
	return &Client{wire: w}, nil
}

func segment(
	value string,
) string {
	return url.PathEscape(value)
}

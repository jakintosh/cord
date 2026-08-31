package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

// DefaultSocketPath is the default server-daemon administration socket.
const DefaultSocketPath = "/var/run/cord/server.sock"

// Client calls the Cord server daemon's administration API.
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
		return nil, errors.New("server admin: socket path required")
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

func marshalJSON(
	value any,
) (
	[]byte,
	error,
) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return body, nil
}

func segment(
	value string,
) string {
	return url.PathEscape(value)
}

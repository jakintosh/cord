package admin

import (
	"encoding/json"
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

type Client struct {
	wire wire.Client
}

func NewClient(
	socketPath string,
) (
	*Client,
	error,
) {
	w, err := wire.NewClient("unix:///"+socketPath, wire.ClientOptions{})
	if err != nil {
		return nil, err
	}
	return &Client{wire: w}, nil
}

func marshalJSON(
	v any,
) (
	[]byte,
	error,
) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return b, nil
}

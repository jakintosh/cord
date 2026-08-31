package client

import "context"

// Peer describes a cached peer joined with live WireGuard state.
type Peer struct {
	Name          string  `json:"name"`
	Route         string  `json:"route"`
	Endpoint      string  `json:"endpoint,omitempty"`
	LastHandshake *string `json:"last_handshake,omitempty"`
	Connected     bool    `json:"connected"`
}

// ListPeers lists the peers visible on a managed network.
func (c *Client) ListPeers(
	ctx context.Context,
	network string,
) (
	[]Peer,
	error,
) {
	var result []Peer
	return result, c.wire.Get(ctx, "/networks/"+segment(network)+"/peers", &result)
}

package server

import "context"

// Peer describes a registered network peer.
type Peer struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Route     string `json:"route"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
}

// UpdatePeerRequest changes mutable peer fields.
type UpdatePeerRequest struct {
	Name    *string `json:"name,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// ListPeers lists a network's peers.
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

// UpdatePeer changes mutable peer fields.
func (c *Client) UpdatePeer(
	ctx context.Context,
	network, peer string,
	newName *string,
	enabled *bool,
) (
	Peer,
	error,
) {
	body, err := marshalJSON(UpdatePeerRequest{
		Name:    newName,
		Enabled: enabled,
	})
	if err != nil {
		return Peer{}, err
	}
	var result Peer
	path := "/networks/" + segment(network) + "/peers/" + segment(peer)
	return result, c.wire.Patch(ctx, path, body, &result)
}

// DeletePeer removes a peer.
func (c *Client) DeletePeer(
	ctx context.Context,
	network, peer string,
) error {
	path := "/networks/" + segment(network) + "/peers/" + segment(peer)
	return c.wire.Delete(ctx, path, nil)
}

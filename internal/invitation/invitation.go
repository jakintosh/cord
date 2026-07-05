// Package invitation defines the invitation wire type: the opaque JSON
// payload the server emits for a pending registration and the client
// redeems to join a network.
package invitation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrInvalid is returned when an invitation payload cannot be parsed.
var ErrInvalid = errors.New("invalid invitation")

// NetworkInfo describes a cord network and how to reach it. It travels
// in invitation payloads and is stored in client-side network config.
type NetworkInfo struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Endpoint    string `json:"endpoint"`     // external WG endpoint
	ServerRoute string `json:"server_route"` // server's host route on the overlay (e.g. "10.42.0.1/32")
	APIPort     uint16 `json:"api_port"`     // server API port on the overlay
}

// PeerIdentity describes a peer's assigned identity on the network.
// The PrivateKey is only present in the initial invitation; it is
// omitted from redemption responses.
type PeerIdentity struct {
	Route      string `json:"route"`
	PrivateKey string `json:"private_key,omitempty"`
}

// Invitation is the opaque JSON payload delivered to a peer. It contains
// everything the peer needs to connect to and authenticate on the invite
// network and redeem a permanent identity.
type Invitation struct {
	Network NetworkInfo  `json:"network"`
	Peer    PeerIdentity `json:"peer"`
}

// Parse reads and validates an Invitation from a JSON reader.
func Parse(
	r io.Reader,
) (
	*Invitation,
	error,
) {
	var inv Invitation
	if err := json.NewDecoder(r).Decode(&inv); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return &inv, nil
}

// Write serializes the invitation as JSON to the writer.
func (inv *Invitation) Write(
	w io.Writer,
) error {
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		return fmt.Errorf("write invitation: %w", err)
	}
	return nil
}

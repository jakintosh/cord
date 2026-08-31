package invite

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrInvalid is returned when an invitation payload cannot be parsed or is
// missing required fields.
var ErrInvalid = errors.New("invalid invitation")

// NetworkInfo describes a Cord network and how to reach it.
type NetworkInfo struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Endpoint    string `json:"endpoint"`
	ServerRoute string `json:"server_route"`
	NetworkCidr string `json:"network_cidr"`
	APIPort     uint16 `json:"api_port"`
}

// PeerIdentity describes a peer's assigned identity on the network. PrivateKey
// is present in the initial invitation and omitted from redemption responses.
type PeerIdentity struct {
	Route      string `json:"route"`
	PrivateKey string `json:"private_key,omitempty"`
}

// Invitation contains the temporary identity and server information needed to
// join a Cord network.
type Invitation struct {
	Network NetworkInfo  `json:"network"`
	Peer    PeerIdentity `json:"peer"`
}

// Parse reads an Invitation from a JSON reader.
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

// Write serializes the invitation as JSON.
func (inv *Invitation) Write(
	w io.Writer,
) error {
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		return fmt.Errorf("write invitation: %w", err)
	}
	return nil
}

// Validate reports whether the invitation carries every field needed by a
// peer. PrivateKey is required when requirePrivateKey is true.
func (inv *Invitation) Validate(
	requirePrivateKey bool,
) error {
	if requirePrivateKey && inv.Peer.PrivateKey == "" {
		return fmt.Errorf("%w: missing peer private key", ErrInvalid)
	}

	switch {
	case inv.Network.Name == "":
		return fmt.Errorf("%w: missing network name", ErrInvalid)
	case inv.Peer.Route == "":
		return fmt.Errorf("%w: missing peer route", ErrInvalid)
	case inv.Network.PublicKey == "":
		return fmt.Errorf("%w: missing server public key", ErrInvalid)
	case inv.Network.Endpoint == "":
		return fmt.Errorf("%w: missing server endpoint", ErrInvalid)
	case inv.Network.ServerRoute == "":
		return fmt.Errorf("%w: missing server route", ErrInvalid)
	case inv.Network.NetworkCidr == "":
		return fmt.Errorf("%w: missing network cidr", ErrInvalid)
	case inv.Network.APIPort == 0:
		return fmt.Errorf("%w: missing server API port", ErrInvalid)
	}
	return nil
}

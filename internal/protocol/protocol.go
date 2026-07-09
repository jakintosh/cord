// Package protocol defines the wire types exchanged between cord clients
// and cord servers over the invite and main networks, plus the
// out-of-band invitation artifact. The invitation format is
// intentionally JSON so it can travel through clipboards, files, and
// chat channels between the operator issuing it and the peer redeeming
// it.
//
// These shapes are the contract between the two sides. They live here,
// defined once, rather than being duplicated on each side where they
// have historically drifted.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrInvalid is returned when an invitation payload cannot be parsed or
// is missing required fields.
var ErrInvalid = errors.New("invalid invitation")

// NetworkInfo describes a cord network and how to reach it. It travels
// in invitation payloads and is stored in client-side network config.
type NetworkInfo struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Endpoint    string `json:"endpoint"`     // external WG endpoint
	ServerRoute string `json:"server_route"` // server's host route on the overlay (e.g. "10.42.0.1/32")
	NetworkCidr string `json:"network_cidr"` // full overlay CIDR (e.g. "10.42.0.0/16")
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

// Write serializes the invitation as JSON to the writer.
func (inv *Invitation) Write(
	w io.Writer,
) error {
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		return fmt.Errorf("write invitation: %w", err)
	}
	return nil
}

// Validate reports whether the invitation carries every field a peer
// needs for the given form. A parseable-but-incomplete invitation is
// still invalid: "is this a complete invitation" is a protocol concern.
// The peer private key is required only for FormInitial; FormRedeemed
// omits it by design.
func (inv *Invitation) Validate(
	reqPrivKey bool,
) error {
	if reqPrivKey && inv.Peer.PrivateKey == "" {
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

// RedeemRequest is the JSON body for POST /redeem on the invite network.
// It provides the caller's new permanent public key. The invite temp key
// is derived server-side from the WireGuard tunnel source IP. A
// successful redemption responds with an Invitation whose PrivateKey is
// omitted.
type RedeemRequest struct {
	PermPubKey string `json:"perm_pubkey"`
}

// VisiblePeer is a peer as seen from the perspective of another peer on
// the main network. It carries identity and recently witnessed
// endpoints, and is returned by GET /peers.
type VisiblePeer struct {
	Name      string            `json:"name"`
	Route     string            `json:"route"`
	PublicKey string            `json:"public_key"`
	Endpoints []EndpointWitness `json:"endpoints"`
}

// EndpointWitness records a peer's endpoint and when it was observed.
type EndpointWitness struct {
	Endpoint  string    `json:"endpoint"`
	Timestamp time.Time `json:"timestamp"`
}

// EndpointSighting reports a locally-observed peer endpoint to the
// server for gossip via POST /endpoints. The server timestamps the
// sighting on receipt and resolves the witness identity from the
// WireGuard source IP, so witness attribution is not part of the wire
// contract.
type EndpointSighting struct {
	PeerKey  string `json:"peer_key"`
	Endpoint string `json:"endpoint"`
}

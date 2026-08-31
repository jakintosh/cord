// Package protocol defines the internal wire types exchanged between Cord
// clients and servers over the invite and main networks. Public out-of-band
// invitation artifacts are owned by package invite.
package protocol

import (
	"time"

	"git.studiopollinator.com/pollinator/cord/pkg/invite"
)

// Deprecated aliases keep the internal peer protocol source-compatible while
// invitation ownership moves to the public package.
var ErrInvalid = invite.ErrInvalid

type NetworkInfo = invite.NetworkInfo
type PeerIdentity = invite.PeerIdentity
type Invitation = invite.Invitation

var ParseInvitation = invite.Parse

// RedeemRequest is the JSON body for POST /redeem on the invite network.
// It provides the caller's new permanent public key. The invite temp key
// is derived server-side from the WireGuard tunnel source IP. A
// successful redemption responds with an Invitation whose PrivateKey is
// omitted.
type RedeemRequest struct {
	PermPubKey string `json:"perm_pubkey"`
}

// VisiblePeer is a peer as seen from the perspective of another peer on
// the main network. It carries identity and recently witnessed endpoints and
// is returned as part of GET /snapshot.
type VisiblePeer struct {
	Name      string            `json:"name"`
	Route     string            `json:"route"`
	PublicKey string            `json:"public_key"`
	Endpoints []EndpointWitness `json:"endpoints"`
}

// VisibleNetworkSnapshot is one complete peer synchronization response.
// Peers and topology are derived from the same server-side topology state.
type VisibleNetworkSnapshot struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Peers       []VisiblePeer `json:"peers"`
	Topology    TopologyView  `json:"topology"`
}

type TopologyView struct {
	Nodes           []TopologyNode        `json:"nodes"`
	Associations    []TopologyAssociation `json:"associations"`
	EffectiveGroups []string              `json:"effective_groups"`
	SubjectPeer     string                `json:"subject_peer"`
}

type TopologyNode struct {
	Name          string   `json:"name"`
	CIDR          string   `json:"cidr"`
	Terminal      bool     `json:"terminal"`
	DisplayParent string   `json:"display_parent,omitempty"`
	Groups        []string `json:"groups"`
	PeerName      string   `json:"peer_name,omitempty"`
	Subject       bool     `json:"subject"`
}

type TopologyAssociation struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

// EndpointWitness records a peer's endpoint and when it was observed.
type EndpointWitness struct {
	Endpoint  string    `json:"endpoint"`
	Timestamp time.Time `json:"timestamp"`
}

// StatusResponse is the generic acknowledgement body for peer API
// mutations such as POST /confirm and POST /endpoints.
type StatusResponse struct {
	Status string `json:"status"`
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

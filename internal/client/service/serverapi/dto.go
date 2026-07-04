package serverapi

import "time"

// --- Peer API (main network) ---

// VisiblePeerDTO is a peer as seen from the perspective of another peer.
// It includes identity and recently witnessed endpoints. Mirrors the
// server's peer.VisiblePeerDTO JSON wire format.
type VisiblePeerDTO struct {
	Name      string               `json:"name"`
	Route     string               `json:"route"`
	PublicKey string               `json:"public_key"`
	Endpoints []EndpointWitnessDTO `json:"endpoints"`
}

// EndpointWitnessDTO records a peer's endpoint and when it was observed.
type EndpointWitnessDTO struct {
	Endpoint  string    `json:"endpoint"`
	Timestamp time.Time `json:"timestamp"`
}

// EndpointSightingDTO reports a locally-observed peer endpoint to the
// server for gossip. The server timestamps the sighting on receipt and
// resolves the witness identity from the WireGuard source IP.
type EndpointSightingDTO struct {
	PeerKey  string `json:"peer_key"`
	Endpoint string `json:"endpoint"`
}

// --- Invite API (invite network) ---

// RedeemInvitationRequest is the JSON body for POST /redeem. It provides
// the caller's new permanent public key. The invite temp key is derived
// server-side from the WireGuard tunnel source IP.
type RedeemInvitationRequest struct {
	PermPubKey string `json:"perm_pubkey"`
}

// InvitationDTO is returned by a successful POST /redeem. It carries
// the permanent network identity that the server assigned to the peer.
type InvitationDTO struct {
	Network NetworkInfoDTO  `json:"network"`
	Peer    PeerIdentityDTO `json:"peer"`
}

// NetworkInfoDTO describes how to reach the coordination server on the
// main network after invite redemption.
type NetworkInfoDTO struct {
	PublicKey   string `json:"public_key"`
	Endpoint    string `json:"endpoint"`
	ServerRoute string `json:"server_route"`
	APIPort     uint16 `json:"api_port"`
}

// PeerIdentityDTO describes the peer's assigned identity on the network.
type PeerIdentityDTO struct {
	Route string `json:"route"`
}

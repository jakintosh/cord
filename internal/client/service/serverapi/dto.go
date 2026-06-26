package serverapi

import "time"

// --- Peer API (main network) ---

// VisiblePeerDTO is a peer as seen from the perspective of another peer.
// It includes identity and recently witnessed endpoints. Mirrors the
// server's peer.VisiblePeerDTO JSON wire format.
type VisiblePeerDTO struct {
	Name      string               `json:"name"`
	Cidr      string               `json:"cidr"`
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

// RedeemInviteRequest is the JSON body for POST /redeem. It provides
// the temporary invite key and the caller's permanent public key.
type RedeemInviteRequest struct {
	TempPubKey string `json:"temp_pubkey"`
	PermPubKey string `json:"perm_pubkey"`
}

// RedeemResultDTO is returned by a successful POST /redeem. It carries
// the permanent network identity that the server assigned to the peer.
type RedeemResultDTO struct {
	NetworkName  string        `json:"network_name"`
	AssignedCidr string        `json:"assigned_cidr"`
	Server       ServerInfoDTO `json:"server"`
}

// ServerInfoDTO describes how to reach the coordination server on the
// main network after invite redemption.
type ServerInfoDTO struct {
	PublicKey        string `json:"public_key"`
	ExternalEndpoint string `json:"external_endpoint"`
	InternalEndpoint string `json:"internal_endpoint"`
}

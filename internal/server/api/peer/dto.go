package peer

import "time"

type VisiblePeerDTO struct {
	Name      string               `json:"name"`
	Cidr      string               `json:"cidr"`
	PublicKey string               `json:"public_key"`
	Endpoints []EndpointWitnessDTO `json:"endpoints"`
}

type EndpointWitnessDTO struct {
	Witness   string    `json:"witness"`
	Endpoint  string    `json:"endpoint"`
	Timestamp time.Time `json:"timestamp"`
}

type EndpointSightingDTO struct {
	WitnessKey string `json:"witness_key"`
	PeerKey    string `json:"peer_key"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

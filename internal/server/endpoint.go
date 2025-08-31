package server

type EndpointWitness struct {
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

type EndpointSighting struct {
	PeerKey    string `json:"peerKey"`
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

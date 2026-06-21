package service

type Peer struct {
	Name         string
	PublicKey    string
	Cidr         string
	Endpoint     string
	EndpointTime int64
}

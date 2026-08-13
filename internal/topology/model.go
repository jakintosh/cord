package topology

import "net"

type Cidr struct {
	Name     string
	Cidr     string
	Base     net.IP
	Last     net.IP
	Prefix   int
	Bits     int
	Terminal bool
}

type Peer struct {
	Name      string
	PublicKey string
	Route     string
}

type Snapshot struct {
	Cidrs        []Cidr
	Assignments  map[string][]string
	Associations map[string]map[string]bool
	PeerCidr     map[string]string
	PeerInfo     map[string]Peer
}

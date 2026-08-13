package topology

import (
	"bytes"
	"net"
)

// Cidr describes one named network range in a topology snapshot.
type Cidr struct {
	Name     string
	Cidr     string
	Base     net.IP
	Last     net.IP
	Prefix   int
	Bits     int
	Terminal bool
}

// Peer contains the configuration needed to reach one network peer.
type Peer struct {
	Name      string
	PublicKey string
	Route     string
}

// Snapshot contains the source data used to compile a topology.
type Snapshot struct {
	Cidrs        []Cidr
	Assignments  map[string][]string
	Associations map[string]map[string]bool
	PeerCidr     map[string]string
	PeerInfo     map[string]Peer
}

func (c Cidr) contains(
	other Cidr,
) bool {
	return c.Bits == other.Bits &&
		bytes.Compare(c.Base, other.Base) <= 0 &&
		bytes.Compare(c.Last, other.Last) >= 0
}

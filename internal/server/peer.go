package server

import (
	"fmt"
	"net"
)

type PeerDesc struct {
	Name    string
	Ip      net.IP
	Admin   bool
	Expires int64
}

type Peer struct {
	PeerId    int64  `json:"peerId"`
	CidrId    int64  `json:"cidrId"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	Cidr      string `json:"cidr"`
	Admin     bool   `json:"admin"`
	Confirmed bool   `json:"confirmed"`
	Disabled  bool   `json:"disabled"`
}

func (p *Peer) String() string {
	return fmt.Sprintf(
		"%s | %s | %s",
		p.PublicKey,
		p.Cidr,
		p.Name,
	)
}

func (ctx *Context) RenamePeer(
	peer string,
	newName string,
) error {
	return ctx.RenameCidr(peer, newName)
}

func (ctx *Context) SetPeerEnabled(
	peer string,
	enabled bool,
) error {
	return ctx.Store.PeerSetEnabled(peer, enabled)
}

func (ctx *Context) CheckPeerExists(
	peerName string,
) bool {
	return ctx.Store.PeerExists(peerName)
}

func (ctx *Context) GetPeersOfPeerNamed(
	peerName string,
) (
	[]Peer,
	error,
) {
	return ctx.Store.PeerListPeers(peerName)
}

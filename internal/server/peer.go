package server

import (
	"fmt"
	"net"
)

type PublicPeer struct {
	Name      string            `json:"name"`
	Cidr      string            `json:"cidr"`
	PublicKey string            `json:"publicKey"`
	Endpoints []EndpointWitness `json:"endpoints"`
}

type Peer struct {
	Name      string `json:"name"`
	Cidr      string `json:"cidr"`
	PublicKey string `json:"publicKey"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
	Confirmed bool   `json:"confirmed"`
}

type CreatePeerRequest struct {
	Name  string `json:"name"`
	Cidr  string `json:"cidr"`
	Admin bool   `json:"admin"`
}

type UpdatePeerRequest struct {
	Name    *string `json:"name,omitempty"`
	Admin   *bool   `json:"admin,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

func (p *Peer) String() string {
	return fmt.Sprintf(
		"%s | %s | %s",
		p.PublicKey,
		p.Cidr,
		p.Name,
	)
}

func (ctx *Context) PeerGetByIP(
	ip net.IP,
) (
	*Peer,
	error,
) {
	return &Peer{
		Name:      "peer",
		PublicKey: "abc123",
		Cidr:      "10.0.0.1/32",
		Admin:     true,
		Confirmed: true,
		Enabled:   false,
	}, nil
}

func (ctx *Context) UpdatePeer(
	peer string,
	req UpdatePeerRequest,
) (
	*Peer,
	error,
) {
	return nil, nil
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

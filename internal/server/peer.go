package server

import (
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

func (ctx *Context) GetPeerByIP(
	ip net.IP,
) (
	*Peer,
	error,
) {
	return ctx.Store.PeerGetByIP(ip)
}

func (ctx *Context) UpdatePeer(
	peer string,
	req UpdatePeerRequest,
) (
	*Peer,
	error,
) {
	result, err := ctx.Store.PeerUpdate(peer, req)
	if err != nil {
		return nil, err
	}

	// TODO: After updating peer in database, should call Interface.Sync()
	// to update the live WireGuard interface with the new peer configuration
	// This requires the server context to maintain references to the
	// main and invite Interface instances created in Serve()

	return result, nil
}

func (ctx *Context) CheckPeerExists(
	peerName string,
) bool {
	return ctx.Store.PeerExists(peerName)
}

func (ctx *Context) GetPeersOfPeerNamed(
	peerName string,
) (
	[]*Peer,
	error,
) {
	return ctx.Store.PeerListPeers(peerName)
}

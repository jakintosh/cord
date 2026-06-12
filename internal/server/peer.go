package server

import (
	"net"
	"time"
)

// endpointTTL is how long an endpoint sighting is considered current.
const endpointTTL = 24 * time.Hour

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

func (ctx *Context) GetPeer(
	name string,
) (
	*Peer,
	error,
) {
	return ctx.Store.PeerGet(name)
}

func (ctx *Context) ListPeers() (
	[]*Peer,
	error,
) {
	return ctx.Store.PeerList()
}

func (ctx *Context) UpdatePeer(
	peer string,
	req UpdatePeerRequest,
) (
	*Peer,
	error,
) {
	return ctx.Store.PeerUpdate(peer, req)
}

func (ctx *Context) DeletePeer(
	peer string,
) error {
	return ctx.Store.PeerDelete(peer)
}

// ConfirmPeer finalizes redemption: the peer has proven it can reach
// the server over the main network from its assigned IP.
func (ctx *Context) ConfirmPeer(
	pubKey string,
	ip net.IP,
) error {
	return ctx.Store.PeerConfirm(pubKey, ip)
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

// GetVisiblePeers returns the peers visible to the named peer along
// with each peer's recently witnessed endpoints, newest first.
func (ctx *Context) GetVisiblePeers(
	peerName string,
) (
	[]*PublicPeer,
	error,
) {
	peers, err := ctx.Store.PeerListPeers(peerName)
	if err != nil {
		return nil, err
	}

	since := time.Now().Add(-endpointTTL).Unix()
	endpoints, err := ctx.Store.EndpointsRecent(since)
	if err != nil {
		return nil, err
	}

	public := make([]*PublicPeer, 0, len(peers))
	for _, peer := range peers {
		public = append(public, &PublicPeer{
			Name:      peer.Name,
			Cidr:      peer.Cidr,
			PublicKey: peer.PublicKey,
			Endpoints: endpoints[peer.PublicKey],
		})
	}

	return public, nil
}

// ReportEndpoints records endpoint sightings witnessed by a peer.
func (ctx *Context) ReportEndpoints(
	sightings []EndpointSighting,
) error {
	return ctx.Store.EndpointReport(sightings)
}

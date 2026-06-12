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

func (srv *Server) GetPeerByIP(
	ip net.IP,
) (
	*Peer,
	error,
) {
	return srv.Store.PeerGetByIP(ip)
}

func (srv *Server) GetPeer(
	name string,
) (
	*Peer,
	error,
) {
	return srv.Store.PeerGet(name)
}

func (srv *Server) ListPeers() (
	[]*Peer,
	error,
) {
	return srv.Store.PeerList()
}

func (srv *Server) UpdatePeer(
	peer string,
	req UpdatePeerRequest,
) (
	*Peer,
	error,
) {
	return srv.Store.PeerUpdate(peer, req)
}

func (srv *Server) DeletePeer(
	peer string,
) error {
	return srv.Store.PeerDelete(peer)
}

// ConfirmPeer finalizes redemption: the peer has proven it can reach
// the server over the main network from its assigned IP.
func (srv *Server) ConfirmPeer(
	pubKey string,
	ip net.IP,
) error {
	return srv.Store.PeerConfirm(pubKey, ip)
}

func (srv *Server) CheckPeerExists(
	peerName string,
) bool {
	return srv.Store.PeerExists(peerName)
}

func (srv *Server) GetPeersOfPeerNamed(
	peerName string,
) (
	[]*Peer,
	error,
) {
	return srv.Store.PeerListPeers(peerName)
}

// GetVisiblePeers returns the peers visible to the named peer along
// with each peer's recently witnessed endpoints, newest first.
func (srv *Server) GetVisiblePeers(
	peerName string,
) (
	[]*PublicPeer,
	error,
) {
	peers, err := srv.Store.PeerListPeers(peerName)
	if err != nil {
		return nil, err
	}

	since := time.Now().Add(-endpointTTL).Unix()
	endpoints, err := srv.Store.EndpointsRecent(since)
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
func (srv *Server) ReportEndpoints(
	sightings []EndpointSighting,
) error {
	return srv.Store.EndpointReport(sightings)
}

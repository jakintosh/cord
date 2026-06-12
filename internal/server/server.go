package server

import (
	"fmt"
	"net"
)

type ServerStore interface {
	Create(name string, root *net.IPNet, serverPubKey string) error
	Delete(name string) error

	AssociationList() ([]*Association, error)
	AssociationCreate(cidr1 string, cidr2 string) error
	AssociationDelete(cidr1 string, cidr2 string) error

	CidrList() ([]*Cidr, error)
	CidrGet(name string) (*Cidr, error)
	CidrCreate(name string, cidr *net.IPNet) error
	CidrUpdate(name string, req UpdateCidrRequest) error
	CidrDelete(name string) error

	EndpointReport(sightings []EndpointSighting) error
	EndpointsRecent(since int64) (map[string][]EndpointWitness, error)
	EndpointsPrune(before int64) error

	InviteList() ([]*ServerInvite, error)
	InviteListActive() ([]*ServerInvite, error)
	InviteGet(name string) (*ServerInvite, error)
	InviteGetByIP(ip net.IP) (*ServerInvite, error)
	InviteGetByIPAny(ip net.IP) (*ServerInvite, error)
	InviteCreate(name string, pubKey string, tempIP net.IP, finalIP net.IP, admin bool, expiration int64) error
	InviteRedeem(pubKey string, newKey string) error
	InvitesPruneExpired(before int64) error

	PeerExists(name string) bool
	PeerList() ([]*Peer, error)
	PeerListPeers(name string) ([]*Peer, error)
	PeerGet(name string) (*Peer, error)
	PeerGetByIP(ip net.IP) (*Peer, error)
	PeerGetByKey(pubKey string) (*Peer, error)
	PeerConfirm(pubKey string, ip net.IP) error
	PeerDelete(name string) error
	PeerUpdate(name string, req UpdatePeerRequest) (*Peer, error)
}

// Server is the cord network service: domain operations over the
// network's config store and state store, both injected at
// construction.
type Server struct {
	Network string
	Config  Config
	Store   ServerStore
}

// Options configures a Server. The stores are built by the caller (the
// composition root) and passed in ready for use.
type Options struct {
	Network string
	Config  Config
	Store   ServerStore
}

// New prepares a server for a network.
func New(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("server requires a config store")
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("server requires a network store")
	}
	return &Server{
		Network: opts.Network,
		Config:  opts.Config,
		Store:   opts.Store,
	}, nil
}

package service

import (
	"net"
	"time"
)

type Store interface {
	GetNetwork(name string) (*Network, error)
	ListNetworkNames() ([]string, error)
	InsertNetwork(network *Network) error
	DeleteNetwork(name string) error

	GetPeer(network, name string) (*Peer, error)
	GetPeerByIP(network string, ip net.IP) (*Peer, error)
	GetPeerByKey(network, pubKey string) (*Peer, error)
	ListPeers(network string) ([]*Peer, error)
	InsertPeer(network string, peer *Peer) error
	DeletePeer(network, name string) error
	UpdatePeer(network, name string, req UpdatePeerRequest) (*Peer, error)
	PeerExists(network, name string) (bool, error)

	GetCidr(network, name string) (*Cidr, error)
	ListCidrs(network string) ([]*Cidr, error)
	InsertCidr(network string, cidr *Cidr) error
	DeleteCidr(network, name string) error
	UpdateCidr(network, name string, req UpdateCidrRequest) (*Cidr, error)

	ListAssociations(network string) ([]*Association, error)
	InsertAssociation(network string, a *Association) error
	DeleteAssociation(network, cidr1, cidr2 string) error

	GetInvite(network, name string) (*Invite, error)
	GetInviteByIP(network string, ip net.IP) (*Invite, error)
	ListInvites(network string) ([]*Invite, error)
	ListActiveInvites(network string) ([]*Invite, error)
	InsertInvite(network string, invite *Invite) error
	RedeemInvite(network string, tempPubKey, permPubKey string) error
	DeleteInvite(network, name string) error
	DeleteExpiredInvites(network string, before time.Time) error

	GetRecentEndpoints(network string, since time.Time) (map[string][]EndpointWitness, error)
	InsertEndpointSightings(network string, sightings []EndpointSighting) error
	DeleteEndpointsBefore(network string, before time.Time) error

	Close() error
}

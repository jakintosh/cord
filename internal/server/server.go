package server

import (
	"fmt"
	"net"
)

type BackendType int

const (
	UndefinedBackend BackendType = iota
	KernelBackend
	UserspaceBackend
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

	EndpointReport(sightings []string) error

	InviteList() ([]*ServerInvite, error)
	InviteGet(name string) (*ServerInvite, error)
	InviteCreate(name string, pubKey string, tempIP net.IP, finalIP net.IP, admin bool, expiration int64) error
	InviteRedeem(pubKey string, newKey string) error

	PeerExists(name string) bool
	PeerList() ([]*Peer, error)
	PeerListPeers(name string) ([]*Peer, error)
	PeerGet(name string) (*Peer, error)
	PeerUpdate(name string, req UpdatePeerRequest) (*Peer, error)
}

type Context struct {
	Name   string
	Config Config
	Store  ServerStore
}

func NewContext(
	network string,
	config Config,
	store ServerStore,
) (*Context, error) {

	ctx := &Context{
		Name:   network,
		Config: config,
		Store:  store,
	}

	return ctx, nil
}

func (ctx *Context) Serve(
	noRouting bool,
	mtu int,
	backend BackendType,
) error {

	fmt.Println("Serve Network")
	fmt.Printf("network: %s\n", ctx.Name)
	fmt.Printf("configDir: %s\n", ctx.Config)
	fmt.Printf("noRouting: %t\n", noRouting)
	fmt.Printf("mtu: %d\n", mtu)
	fmt.Printf("backend: %v\n", backend)

	return nil
}

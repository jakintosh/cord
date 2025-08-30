package server

import (
	"fmt"
	"net"

	_ "modernc.org/sqlite"
)

type BackendType int

const (
	UndefinedBackend BackendType = iota
	KernelBackend
	UserspaceBackend
)

type ServerStore interface {
	Delete(name string) error

	AssociationCreate(a string, b string) error
	AssociationListAssociatedCidrIds(id int64) ([]int64, error)
	AssociationDelete(a string, b string) error

	CidrCreateRoot(name string, cidr *net.IPNet) error
	CidrCreate(name string, cidr *net.IPNet) error
	CidrRename(name string, newName string) error
	CidrDelete(name string) error

	EndpointReport(sightings []string) error

	InviteCreate(name string, pubKey string, cidr string, admin bool, inviteExpires int64) error
	InviteRedeem(pubKey string, newKey string) error

	PeerExists(name string) bool
	PeerRename(name string, newName string) error
	PeerSetAdmin(name string, admin bool) error
	PeerSetEnabled(name string, enabled bool) error
	PeerListPeers(name string) ([]Peer, error)
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

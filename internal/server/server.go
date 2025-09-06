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
	InviteGetByIP(ip net.IP) (*ServerInvite, error)
	InviteCreate(name string, pubKey string, tempIP net.IP, finalIP net.IP, admin bool, expiration int64) error
	InviteRedeem(pubKey string, newKey string) error

	PeerExists(name string) bool
	PeerList() ([]*Peer, error)
	PeerListPeers(name string) ([]*Peer, error)
	PeerGet(name string) (*Peer, error)
	PeerGetByIP(ip net.IP) (*Peer, error)
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
	fmt.Printf("noRouting: %t\n", noRouting)
	fmt.Printf("mtu: %d\n", mtu)
	fmt.Printf("backend: %v\n", backend)

	// TODO: Implement proper server interface management
	// According to the documentation, the serve function should:
	// 1. Read server configuration from ctx.Config
	// 2. Create main network interface using wireguard.NewInterface()
	// 3. Create invite network interface using wireguard.NewInterface()
	// 4. Bring up both interfaces using Interface.Up()
	// 5. Start HTTP API server
	// 6. Handle peer updates using Interface.Sync()

	fmt.Println("TODO: Implement actual WireGuard interface management")
	fmt.Println("This should create and manage both main and invite interfaces")

	return nil
}

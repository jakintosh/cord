package server

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/utils"
)

type NetworkStore interface {
	Delete(name string) error

	AssociationCreate(a string, b string) error
	AssociationListAssociatedCidrIds(id int64) ([]int64, error)
	AssociationDelete(a string, b string) error

	CidrCreateRoot(name string, cidr *net.IPNet) error
	CidrCreate(name string, cidr *net.IPNet) error
	CidrRename(name string, newName string) error
	CidrDelete(name string) error

	InviteCreate(name string, pubKey string, cidr string, admin bool, inviteExpires int64) error
	InviteRedeem(pubKey string, newKey string) error

	PeerExists(name string) bool
	PeerRename(name string, newName string) error
	PeerSetAdmin(name string, admin bool) error
	PeerSetEnabled(name string, enabled bool) error
	PeerListPeers(name string) ([]Peer, error)
}

type NetworkDesc struct {
	Name string
	Cidr string
	Ip   net.IP
	Port uint16
}

func (ctx *Context) CreateNetwork(
	cidr *net.IPNet,
	address net.IP,
	port uint16,
) error {

	if err := utils.ValidateHostName(ctx.Name); err != nil {
		return fmt.Errorf("failed to validate network name: %w", err)
	}

	// make sure we get file handle before we do all the db work
	fileName := ctx.Name + ".toml"
	cfgFile, err := ctx.Config.GetConfigWriter(fileName)
	if err != nil {
		return fmt.Errorf("failed to create config writer: %w", err)
	}

	if err := ctx.CreateRootCidr(cidr); err != nil {
		return fmt.Errorf("failed to add root cidr: %w", err)
	}

	serverIp := utils.GetFirstAssignableIpFromCidr(cidr)

	deviceCfg, peerCfg, err := ctx.CreateInvite("cord-server", serverIp, true, 0)
	if err != nil {
		return fmt.Errorf("failed to add server peer: %w", err)
	}

	deviceCfg.ListenPort = port

	pubKey := peerCfg.PublicKey.String()
	if err := ctx.RedeemInvite(pubKey, pubKey); err != nil {
		return fmt.Errorf("failed to redeem server peer: %w", err)
	}

	if err := deviceCfg.Write(cfgFile); err != nil {
		return fmt.Errorf("failed to write device config file")
	}

	return nil
}

func (ctx *Context) DeleteNetwork() error {

	return ctx.Store.Delete(ctx.Name)
}

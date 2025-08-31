package server_test

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

type NetworkDesc struct {
	Name string
	Cidr string
	Ip   net.IP
	Port uint16
}

type CidrDesc struct {
	Name string
	Cidr string
}

type PeerDesc struct {
	Name  string
	Ip    net.IP
	Admin bool
}

var testNetwork = NetworkDesc{
	Name: "test-network",
	Cidr: "10.0.0.0/16",
	Ip:   net.IPv4(1, 1, 1, 1),
	Port: 10000,
}

var infraCidr = CidrDesc{
	Name: "infra",
	Cidr: "10.0.0.0/17",
}
var fleetCidr = CidrDesc{
	Name: "fleet",
	Cidr: "10.0.128.0/17",
}

var cordServerPeer = PeerDesc{
	Name: "cord-server",
	Ip:   net.IPv4(10, 0, 0, 1),
}
var testServer = PeerDesc{
	Name: "test-server",
	Ip:   net.IPv4(10, 0, 64, 1),
}
var testServer2 = PeerDesc{
	Name: "test-server-2",
	Ip:   net.IPv4(10, 0, 64, 2),
}
var testServer3 = PeerDesc{
	Name: "test-server-3",
	Ip:   net.IPv4(10, 0, 64, 3),
}
var testUser = PeerDesc{
	Name: "test-user",
	Ip:   net.IPv4(10, 0, 128, 1),
}
var testUser2 = PeerDesc{
	Name: "test-user-2",
	Ip:   net.IPv4(10, 0, 128, 2),
}
var testUser3 = PeerDesc{
	Name: "test-user-3",
	Ip:   net.IPv4(10, 0, 128, 3),
}

// func TestRenameCidr()
// func TestRenamePeer()

func createBaseNetwork() (
	*server.Context,
	error,
) {
	store, err := database.Init(testNetwork.Name, ":memory:", false)
	if err != nil {
		return nil, fmt.Errorf("failed to init databse: %w", err)
	}
	ctx, err := server.NewContext(testNetwork.Name, server.NewMemConfig(), store)
	if err != nil {
		return nil, fmt.Errorf("failed to init test server: %v", err)
	}

	if err := addNetwork(ctx, testNetwork); err != nil {
		return nil, err
	}

	if err := addCidr(ctx, infraCidr); err != nil {
		return nil, err
	}

	if err := addCidr(ctx, fleetCidr); err != nil {
		return nil, err
	}

	return ctx, nil
}

func addNetwork(
	c *server.Context,
	desc NetworkDesc,
) error {
	_, cidr, _ := net.ParseCIDR(desc.Cidr)
	if err := c.CreateNetwork(cidr, desc.Ip, desc.Port); err != nil {
		return fmt.Errorf("failed to create network '%s': %v", desc.Name, err)
	}
	return nil
}

func addCidr(
	c *server.Context,
	desc CidrDesc,
) error {
	req := server.CreateCidrRequest{
		Name: desc.Name,
		Cidr: desc.Cidr,
	}
	if err := c.CreateCidr(req); err != nil {
		return fmt.Errorf("failed to create cidr '%s': %v", desc.Name, err)
	}
	return nil
}

func addPeer(
	c *server.Context,
	desc PeerDesc,
) (string, error) {
	_, cfg, err := c.CreateInvite(desc.Name, desc.Ip, desc.Admin, 0)
	if err != nil {
		return "", fmt.Errorf("failed to create peer '%s': %v", desc.Name, err)
	}
	return cfg.PublicKey.String(), nil
}

func redeemPeer(
	c *server.Context,
	key string,
) error {
	if err := c.RedeemInvite(key, key); err != nil {
		return fmt.Errorf("failed to redeem peer for key '%s': %v", key, err)
	}
	return nil
}

func addAndRedeemPeer(
	c *server.Context,
	desc PeerDesc,
) error {
	_, cfg, err := c.CreateInvite(desc.Name, desc.Ip, desc.Admin, 0)
	if err != nil {
		return fmt.Errorf("failed to create peer '%s': %v", desc.Name, err)
	}
	pubKey := cfg.PublicKey.String()
	if err := c.RedeemInvite(pubKey, pubKey); err != nil {
		return fmt.Errorf("failed to redeem peer for key '%s': %v", pubKey, err)
	}
	return nil
}

func expectPeerCount(
	c *server.Context,
	desc PeerDesc,
	count int,
) error {
	peers, err := c.GetPeersOfPeerNamed(desc.Name)
	if err != nil {
		return fmt.Errorf("failed to get peers for peer: %v", err)
	}
	if len(peers) != count {
		return fmt.Errorf("expected %d peers for 'node', found %d: %v", count, len(peers), peers)
	}
	return nil
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }

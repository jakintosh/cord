package server

import (
	"fmt"
	"net"
	"testing"
)

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

func TestPeerRedeem(t *testing.T) {

	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	key, err := addPeer(ctx, testServer)
	if err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 0); err != nil {
		t.Fatal(err)
	}

	if err := redeemPeer(ctx, key); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 1); err != nil {
		t.Fatal(err)
	}
}

func TestPeerEnable(t *testing.T) {

	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	if err := addAndRedeemPeer(ctx, testServer); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 1); err != nil {
		t.Fatal(err)
	}

	if err := ctx.SetPeerEnabled(testServer.Name, false); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 0); err != nil {
		t.Fatal(err)
	}
}

func TestPeerAssociations(t *testing.T) {

	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	if err := addAndRedeemPeer(ctx, testServer); err != nil {
		t.Fatal(err)
	}

	if err := addAndRedeemPeer(ctx, testUser); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 0); err != nil {
		t.Fatal(err)
	}

	if err := ctx.CreateAssociation("fleet", "infra"); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 2); err != nil {
		t.Fatal(err)
	}

	if err := ctx.DeleteAssociation("fleet", "infra"); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 0); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteCidr(t *testing.T) {

	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	if err := addAndRedeemPeer(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if err := addAndRedeemPeer(ctx, testUser2); err != nil {
		t.Fatal(err)
	}
	if err := addAndRedeemPeer(ctx, testUser3); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 2); err != nil {
		t.Fatal(err)
	}

	if err := ctx.DeleteCidr(fleetCidr.Name); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 3); err != nil {
		t.Fatal(err)
	}
}

// func TestRenameCidr()
// func TestRenamePeer()

func createBaseNetwork() (
	*Context,
	error,
) {
	ctx, err := NewMemContext(testNetwork.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create test context: %v", err)
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
	c *Context,
	desc NetworkDesc,
) error {
	_, cidr, _ := net.ParseCIDR(desc.Cidr)
	if err := c.CreateNetwork(cidr, desc.Ip, desc.Port); err != nil {
		return fmt.Errorf("failed to create network '%s': %v", desc.Name, err)
	}
	return nil
}

func addCidr(
	c *Context,
	desc CidrDesc,
) error {
	_, cidr, err := net.ParseCIDR(desc.Cidr)
	if err != nil {
		return fmt.Errorf("failed to parse cidr '%s': %v", desc.Cidr, err)
	}
	if err := c.CreateCidr(desc.Name, cidr); err != nil {
		return fmt.Errorf("failed to create cidr '%s': %v", desc.Name, err)
	}
	return nil
}

func addPeer(
	c *Context,
	desc PeerDesc,
) (string, error) {
	_, cfg, err := c.CreatePeer(desc.Name, desc.Ip, desc.Admin, desc.Expires)
	if err != nil {
		return "", fmt.Errorf("failed to create peer '%s': %v", desc.Name, err)
	}
	return cfg.PublicKey.String(), nil
}

func redeemPeer(
	c *Context,
	key string,
) error {
	if err := c.RedeemPeer(key, key); err != nil {
		return fmt.Errorf("failed to redeem peer for key '%s': %v", key, err)
	}
	return nil
}

func addAndRedeemPeer(
	c *Context,
	desc PeerDesc,
) error {
	_, cfg, err := c.CreatePeer(desc.Name, desc.Ip, desc.Admin, desc.Expires)
	if err != nil {
		return fmt.Errorf("failed to create peer '%s': %v", desc.Name, err)
	}
	pubKey := cfg.PublicKey.String()
	if err := c.RedeemPeer(pubKey, pubKey); err != nil {
		return fmt.Errorf("failed to redeem peer for key '%s': %v", pubKey, err)
	}
	return nil
}

func expectPeerCount(
	c *Context,
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

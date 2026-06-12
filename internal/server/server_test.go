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

const testInviteCidr = "172.16.10.0/24"

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
	*server.Server,
	error,
) {
	store, err := database.OpenServer(database.Options{
		Name: testNetwork.Name,
		Dir:  ":memory:",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open databse: %w", err)
	}
	ctx, err := server.New(server.Options{
		Network: testNetwork.Name,
		Config:  server.NewMemConfig(),
		Store:   store,
	})
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
	c *server.Server,
	desc NetworkDesc,
) error {
	_, cidr, _ := net.ParseCIDR(desc.Cidr)
	_, inviteCidr, _ := net.ParseCIDR(testInviteCidr)
	req := server.CreateNetworkRequest{
		RootCidr:   cidr,
		InviteCidr: inviteCidr,
		ExternalIP: desc.Ip,
		ListenPort: desc.Port,
		InvitePort: desc.Port + 1,
		ApiPort:    desc.Port,
	}
	if err := c.CreateNetwork(req); err != nil {
		return fmt.Errorf("failed to create network '%s': %v", desc.Name, err)
	}
	return nil
}

func addCidr(
	c *server.Server,
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
	c *server.Server,
	desc PeerDesc,
) (string, error) {
	req := server.CreateInviteRequest{
		Name:  desc.Name,
		IP:    desc.Ip,
		Admin: desc.Admin,
	}
	invite, err := c.CreateInvite(req)
	if err != nil {
		return "", fmt.Errorf("failed to create peer '%s': %v", desc.Name, err)
	}

	// Return the temporary cidr that can be used for redemption
	return invite.Interface.AssignedCidr, nil
}

func redeemPeer(
	c *server.Server,
	cidr string,
) error {
	// parse ip from cidr
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("failed to parse cidr %s: %v", cidr, err)
	}

	// get invite from ip
	invite, err := c.Store.InviteGetByIP(ip)
	if err != nil {
		return fmt.Errorf("failed to get peer for ip %v: %v", ip, err)
	}

	// redeem invite with invite's public key standing in for a
	// client-generated permanent key
	result, err := c.RedeemInvite(invite, invite.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to redeem invite for key '%s': %v", invite.PublicKey, err)
	}

	// confirm the peer from its assigned main-network IP
	finalIP, _, err := net.ParseCIDR(result.AssignedCidr)
	if err != nil {
		return fmt.Errorf("failed to parse assigned cidr %s: %v", result.AssignedCidr, err)
	}
	if err := c.ConfirmPeer(invite.PublicKey, finalIP); err != nil {
		return fmt.Errorf("failed to confirm peer: %v", err)
	}
	return nil
}

func addAndRedeemPeer(
	c *server.Server,
	desc PeerDesc,
) error {
	cidr, err := addPeer(c, desc)
	if err != nil {
		return fmt.Errorf("failed to add peer: %v", err)
	}
	return redeemPeer(c, cidr)
}

func expectPeerCount(
	c *server.Server,
	desc PeerDesc,
	count int,
) error {
	peers, err := c.GetPeersOfPeerNamed(desc.Name)
	if err != nil {
		return fmt.Errorf("failed to get peers for peer: %v", err)
	}
	if len(peers) != count {
		return fmt.Errorf("expected %d peers for '%s', found %d: %v", count, desc.Name, len(peers), peers)
	}
	return nil
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }

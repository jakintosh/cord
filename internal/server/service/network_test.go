package service_test

import (
	"context"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestCreateNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	net, err := env.Service.CreateNetwork(
		"mynet",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	if net.Name != "mynet" {
		t.Errorf("name = %q, want mynet", net.Name)
	}
	if net.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if net.PublicKey == "" {
		t.Error("public_key should not be empty")
	}
	if net.MainCidr != "10.0.0.0/16" {
		t.Errorf("main_cidr = %q, want 10.0.0.0/16", net.MainCidr)
	}
	if net.InviteCidr != "10.1.0.0/24" {
		t.Errorf("invite_cidr = %q, want 10.1.0.0/24", net.InviteCidr)
	}
	if net.ExternalIP != "1.2.3.4" {
		t.Errorf("external_ip = %q, want 1.2.3.4", net.ExternalIP)
	}
	if net.MainWireguardPort != 51820 {
		t.Errorf("listen_port = %d, want 51820", net.MainWireguardPort)
	}
	if net.InviteWireguardPort != 51821 {
		t.Errorf("invite_listen_port = %d, want 51821", net.InviteWireguardPort)
	}
	if net.MainApiPort != 80 {
		t.Errorf("api_port = %d, want 80", net.MainApiPort)
	}
	if net.InviteApiPort != 80 {
		t.Errorf("invite_api_port = %d, want 80", net.InviteApiPort)
	}
	if net.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

func TestCreateNetwork_StoresKeyPair(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	net, err := env.Service.CreateNetwork(
		"keytest",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	got, err := env.Service.GetNetwork("keytest")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}

	if got.PrivateKey != net.PrivateKey {
		t.Errorf("stored private_key = %q, want %q", got.PrivateKey, net.PrivateKey)
	}
	if got.PublicKey != net.PublicKey {
		t.Errorf("stored public_key = %q, want %q", got.PublicKey, net.PublicKey)
	}
}

func TestCreateNetwork_DuplicateName(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	first, err := env.Service.CreateNetwork(
		"dup",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	_ = first

	_, err = env.Service.CreateNetwork(
		"dup",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Errorf("err = %v, want ErrNetworkExists", err)
	}
}

func TestCreateNetwork_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	_, err := env.Service.CreateNetwork(
		"",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_InvalidMainCIDR(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	_, err := env.Service.CreateNetwork(
		"badcidr",
		"1.2.3.4",
		"not-a-cidr",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_InvalidInviteCIDR(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "not-a-cidr"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	_, err := env.Service.CreateNetwork(
		"badinvite",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_MissingExternalIP(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	_, err := env.Service.CreateNetwork(
		"noip",
		"",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_DefaultPorts(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	inviteCidr := "10.1.0.0/24"

	nw, err := env.Service.CreateNetwork(
		"defaults",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		nil,
		nil,
		&inviteCidr,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if nw.InviteWireguardPort != 51821 {
		t.Errorf("invite_listen_port = %d, want 51821", nw.InviteWireguardPort)
	}
	if nw.MainApiPort != 80 {
		t.Errorf("api_port = %d, want 80", nw.MainApiPort)
	}
	if nw.InviteApiPort != 80 {
		t.Errorf("invite_api_port = %d, want 80", nw.InviteApiPort)
	}
}

func TestCreateNetwork_DefaultInviteCidr(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)

	nw, err := env.Service.CreateNetwork(
		"auto-invite",
		"1.2.3.4",
		"10.27.0.0/16",
		nil,
		&mainWgPort,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if nw.InviteCidr != "172.16.10.0/24" {
		t.Errorf("invite_cidr = %q, want 172.16.10.0/24", nw.InviteCidr)
	}
}

func TestCreateNetwork_OverlappingCIDRs(t *testing.T) {
	env := testutil.SetupService(t)

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteCidr := "10.0.1.0/24"
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)

	_, err := env.Service.CreateNetwork(
		"overlap",
		"1.2.3.4",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
	)
	if !errors.Is(err, service.ErrCIDROverlap) {
		t.Errorf("err = %v, want ErrCIDROverlap", err)
	}
}

func TestGetNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	net, err := env.Service.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if net.Name != "testnet" {
		t.Errorf("name = %q, want testnet", net.Name)
	}
	if net.MainCidr != "10.0.0.0/16" {
		t.Errorf("main_cidr = %q, want 10.0.0.0/16", net.MainCidr)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks_Success(t *testing.T) {
	env := testutil.SetupService(t)

	names, err := env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names, got %d", len(names))
	}

	testutil.SeedNetwork(t, env.Service)

	names, err = env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 1 || names[0] != "testnet" {
		t.Errorf("names = %v, want [testnet]", names)
	}
}

func TestDeleteNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.DeleteNetwork("testnet"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := env.Service.GetNetwork("testnet")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.DeleteNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetwork_CascadesResources(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := "10.0.0.5"
	admin := false
	_, err := env.Service.AddPeer("testnet", "alice", &ip, admin)
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}

	if err := env.Service.DeleteNetwork("testnet"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	peers, err := env.Service.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers after delete: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after cascade, got %d", len(peers))
	}
}

func TestStartNetwork_UsesNetworkPrefixInterfaceAddresses(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	var mainCfg *wireguard.DeviceConfig
	for _, c := range env.Backend.CreateCalls {
		if c.Name == "testnet" {
			mainCfg = &c
			break
		}
	}
	if mainCfg == nil {
		t.Fatal("expected main device")
	}
	if mainCfg.Address.String() != "10.0.0.1/16" {
		t.Fatalf("main address = %q, want 10.0.0.1/16", mainCfg.Address.String())
	}

	var inviteCfg *wireguard.DeviceConfig
	for _, c := range env.Backend.CreateCalls {
		if c.Name == "testnet-i" {
			inviteCfg = &c
			break
		}
	}
	if inviteCfg == nil {
		t.Fatal("expected invite device")
	}
	if inviteCfg.Address.String() != "10.1.0.1/24" {
		t.Fatalf("invite address = %q, want 10.1.0.1/24", inviteCfg.Address.String())
	}
}

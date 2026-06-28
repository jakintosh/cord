package service_test

import (
	"context"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestCreateNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)

	net, err := env.Service.CreateNetwork(service.Network{
		Name:             "mynet",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
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
	if net.ListenPort != 51820 {
		t.Errorf("listen_port = %d, want 51820", net.ListenPort)
	}
	if net.InviteListenPort != 51821 {
		t.Errorf("invite_listen_port = %d, want 51821", net.InviteListenPort)
	}
	if net.ApiPort != 8080 {
		t.Errorf("api_port = %d, want 8080", net.ApiPort)
	}
	if net.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

func TestCreateNetwork_StoresKeyPair(t *testing.T) {
	env := testutil.SetupService(t)

	net, err := env.Service.CreateNetwork(service.Network{
		Name:             "keytest",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
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

	cfg := service.Network{
		Name:             "dup",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	}

	if _, err := env.Service.CreateNetwork(cfg); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := env.Service.CreateNetwork(cfg)
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Errorf("err = %v, want ErrNetworkExists", err)
	}
}

func TestCreateNetwork_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(service.Network{
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_InvalidMainCIDR(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(service.Network{
		Name:             "badcidr",
		MainCidr:         "not-a-cidr",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_InvalidInviteCIDR(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(service.Network{
		Name:             "badinvite",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "not-a-cidr",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_MissingExternalIP(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(service.Network{
		Name:             "noip",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_MissingPorts(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(service.Network{
		Name:             "noports",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("missing listen port: err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_DefaultPorts(t *testing.T) {
	env := testutil.SetupService(t)

	nw, err := env.Service.CreateNetwork(service.Network{
		Name:       "defaults",
		MainCidr:   "10.0.0.0/16",
		InviteCidr: "10.1.0.0/24",
		ExternalIP: "1.2.3.4",
		ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if nw.InviteListenPort != 51821 {
		t.Errorf("invite_listen_port = %d, want 51821", nw.InviteListenPort)
	}
	if nw.ApiPort != 51822 {
		t.Errorf("api_port = %d, want 51822", nw.ApiPort)
	}
}

func TestCreateNetwork_DefaultInviteCidr(t *testing.T) {
	env := testutil.SetupService(t)

	nw, err := env.Service.CreateNetwork(service.Network{
		Name:       "auto-invite",
		MainCidr:   "10.27.0.0/16",
		ExternalIP: "1.2.3.4",
		ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if nw.InviteCidr != "172.16.10.0/24" {
		t.Errorf("invite_cidr = %q, want 172.16.10.0/24", nw.InviteCidr)
	}
}

func TestCreateNetwork_OverlappingCIDRs(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(service.Network{
		Name:             "overlap",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.0.1.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
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

	_, err := env.Service.AddPeer("testnet", service.PeerConfig{
		Name:  "alice",
		IP:    "10.0.0.5",
		Admin: false,
	})
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

	main := env.WireGuard.Devices["testnet"]
	if main == nil {
		t.Fatal("expected main device")
	}
	if main.Address != "10.0.0.1/16" {
		t.Fatalf("main address = %q, want 10.0.0.1/16", main.Address)
	}

	invite := env.WireGuard.Devices["testnet-i"]
	if invite == nil {
		t.Fatal("expected invite device")
	}
	if invite.Address != "10.1.0.1/24" {
		t.Fatalf("invite address = %q, want 10.1.0.1/24", invite.Address)
	}
}

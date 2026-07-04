package service_test

import (
	"errors"
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestCreateNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)

	net, err := env.Service.CreateNetwork(
		"mynet",
		"1.2.3.4",
		service.PlaneConfig{
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       80,
		},
		service.PlaneConfig{
			Cidr:          "10.1.0.0/24",
			WireguardPort: 51821,
			ApiPort:       80,
		},
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
	if net.Main.Cidr != "10.0.0.0/16" {
		t.Errorf("main.cidr = %q, want 10.0.0.0/16", net.Main.Cidr)
	}
	if net.Invite.Cidr != "10.1.0.0/24" {
		t.Errorf("invite.cidr = %q, want 10.1.0.0/24", net.Invite.Cidr)
	}
	if net.ExternalIP != "1.2.3.4" {
		t.Errorf("external_ip = %q, want 1.2.3.4", net.ExternalIP)
	}
	if net.Main.WireguardPort != 51820 {
		t.Errorf("main.wg_port = %d, want 51820", net.Main.WireguardPort)
	}
	if net.Invite.WireguardPort != 51821 {
		t.Errorf("invite.wg_port = %d, want 51821", net.Invite.WireguardPort)
	}
	if net.Main.ApiPort != 80 {
		t.Errorf("main.api_port = %d, want 80", net.Main.ApiPort)
	}
	if net.Invite.ApiPort != 80 {
		t.Errorf("invite.api_port = %d, want 80", net.Invite.ApiPort)
	}
	if net.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

func TestCreateNetwork_StoresKeyPair(t *testing.T) {
	env := testutil.SetupService(t)

	net, err := env.Service.CreateNetwork(
		"keytest",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
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

	_, err := env.Service.CreateNetwork(
		"dup",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
	)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = env.Service.CreateNetwork(
		"dup",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
	)
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Errorf("err = %v, want ErrNetworkExists", err)
	}
}

func TestCreateNetwork_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(
		"",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_InvalidMainCIDR(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(
		"badcidr",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "not-a-cidr", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_InvalidInviteCIDR(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(
		"badinvite",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "not-a-cidr", WireguardPort: 51821, ApiPort: 80},
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_MissingExternalIP(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(
		"noip",
		"",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
	)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateNetwork_DefaultPorts(t *testing.T) {
	env := testutil.SetupService(t)

	nw, err := env.Service.CreateNetwork(
		"defaults",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16"},
		service.PlaneConfig{Cidr: "10.1.0.0/24"},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if nw.Main.WireguardPort != 51820 {
		t.Errorf("main.wg_port = %d, want 51820", nw.Main.WireguardPort)
	}
	if nw.Invite.WireguardPort != 51821 {
		t.Errorf("invite.wg_port = %d, want 51821", nw.Invite.WireguardPort)
	}
	if nw.Main.ApiPort != 80 {
		t.Errorf("main.api_port = %d, want 80", nw.Main.ApiPort)
	}
	if nw.Invite.ApiPort != 80 {
		t.Errorf("invite.api_port = %d, want 80", nw.Invite.ApiPort)
	}
}

func TestCreateNetwork_DefaultInviteCidr(t *testing.T) {
	env := testutil.SetupService(t)

	nw, err := env.Service.CreateNetwork(
		"auto-invite",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.27.0.0/16", WireguardPort: 51820},
		service.PlaneConfig{},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if nw.Invite.Cidr != "172.16.10.0/24" {
		t.Errorf("invite.cidr = %q, want 172.16.10.0/24", nw.Invite.Cidr)
	}
}

func TestCreateNetwork_OverlappingCIDRs(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(
		"overlap",
		"1.2.3.4",
		service.PlaneConfig{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
		service.PlaneConfig{Cidr: "10.0.1.0/24", WireguardPort: 51821, ApiPort: 80},
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
	if net.Main.Cidr != "10.0.0.0/16" {
		t.Errorf("main.cidr = %q, want 10.0.0.0/16", net.Main.Cidr)
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
	parsedIP := net.ParseIP(ip)
	_, err := env.Service.CreateRegistration("testnet", "alice", &parsedIP, false, nil)
	if err != nil {
		t.Fatalf("create registration: %v", err)
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

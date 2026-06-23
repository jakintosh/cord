package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func TestCreateNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)

	net, err := env.svc.CreateNetwork(service.Network{
		Name:             "mynet",
		RootCidr:         "10.0.0.0/16",
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
	if net.RootCidr != "10.0.0.0/16" {
		t.Errorf("root_cidr = %q, want 10.0.0.0/16", net.RootCidr)
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
	env := setupTestEnv(t)

	net, err := env.svc.CreateNetwork(service.Network{
		Name:             "keytest",
		RootCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	got, err := env.svc.GetNetwork("keytest")
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
	env := setupTestEnv(t)

	cfg := service.Network{
		Name:             "dup",
		RootCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.2.3.4",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	}

	if _, err := env.svc.CreateNetwork(cfg); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := env.svc.CreateNetwork(cfg)
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Errorf("err = %v, want ErrNetworkExists", err)
	}
}

func TestCreateNetwork_EmptyName(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.CreateNetwork(service.Network{
		RootCidr:         "10.0.0.0/16",
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

func TestCreateNetwork_InvalidRootCIDR(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.CreateNetwork(service.Network{
		Name:             "badcidr",
		RootCidr:         "not-a-cidr",
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
	env := setupTestEnv(t)

	_, err := env.svc.CreateNetwork(service.Network{
		Name:             "badinvite",
		RootCidr:         "10.0.0.0/16",
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
	env := setupTestEnv(t)

	_, err := env.svc.CreateNetwork(service.Network{
		Name:             "noip",
		RootCidr:         "10.0.0.0/16",
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
	env := setupTestEnv(t)

	_, err := env.svc.CreateNetwork(service.Network{
		Name:             "noports",
		RootCidr:         "10.0.0.0/16",
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
	env := setupTestEnv(t)

	nw, err := env.svc.CreateNetwork(service.Network{
		Name:       "defaults",
		RootCidr:   "10.0.0.0/16",
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
	env := setupTestEnv(t)

	nw, err := env.svc.CreateNetwork(service.Network{
		Name:       "auto-invite",
		RootCidr:   "10.27.0.0/16",
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
	env := setupTestEnv(t)

	_, err := env.svc.CreateNetwork(service.Network{
		Name:             "overlap",
		RootCidr:         "10.0.0.0/16",
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
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	net, err := env.svc.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if net.Name != "testnet" {
		t.Errorf("name = %q, want testnet", net.Name)
	}
	if net.RootCidr != "10.0.0.0/16" {
		t.Errorf("root_cidr = %q, want 10.0.0.0/16", net.RootCidr)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks_Success(t *testing.T) {
	env := setupTestEnv(t)

	names, err := env.svc.ListNetworks()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names, got %d", len(names))
	}

	seedNetwork(t, env.svc)

	names, err = env.svc.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 1 || names[0] != "testnet" {
		t.Errorf("names = %v, want [testnet]", names)
	}
}

func TestDeleteNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	if err := env.svc.DeleteNetwork("testnet"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := env.svc.GetNetwork("testnet")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetwork_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	err := env.svc.DeleteNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetwork_CascadesResources(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name:  "alice",
		IP:    "10.0.0.5",
		Admin: false,
	})
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}

	if err := env.svc.DeleteNetwork("testnet"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	peers, err := env.svc.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers after delete: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after cascade, got %d", len(peers))
	}
}

package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestGetNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "mynet")

	nw, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if nw.Name != "mynet" {
		t.Errorf("name = %q, want mynet", nw.Name)
	}
	if nw.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if pub, _ := wireguard.PublicKey(nw.PrivateKey); pub == "" {
		t.Error("public_key should not be empty")
	}
	if nw.Server.PublicKey == "" {
		t.Errorf("server_pubkey = %q, should not be empty", nw.Server.PublicKey)
	}
	if nw.Server.Endpoint != "1.2.3.4:51820" {
		t.Errorf("server_endpoint = %q, want 1.2.3.4:51820", nw.Server.Endpoint)
	}
	if nw.Server.Route != "10.42.0.1/32" {
		t.Errorf("server_route = %q, want 10.42.0.1/32", nw.Server.Route)
	}
	if nw.Server.APIPort != 8443 {
		t.Errorf("server_api_port = %d, want 8443", nw.Server.APIPort)
	}
	if nw.Enabled {
		t.Error("new network should be disabled")
	}
	if nw.CreatedAt.Unix() != testutil.FixedTime.Unix() {
		t.Errorf("created_at = %v, want %v", nw.CreatedAt, testutil.FixedTime)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks_Empty(t *testing.T) {
	env := testutil.SetupService(t)

	networks, err := env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(networks) != 0 {
		t.Errorf("expected 0 networks, got %d", len(networks))
	}
}

func TestListNetworks_WithNetworks(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "alpha")
	testutil.SeedNetworkDirect(t, env.Database, "beta")

	networks, err := env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(networks))
	}
}

// TestSetNetworkEnabled_PersistsIntent verifies that the enabled flag is
// the whole of the operation: it is written unconditionally and the
// runtime is woken to converge toward it.
func TestSetNetworkEnabled_PersistsIntent(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "enable-me")

	if err := env.Service.SetNetworkEnabled("enable-me", true); err != nil {
		t.Fatalf("enable: %v", err)
	}

	nw, err := env.Service.GetNetwork("enable-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !nw.Enabled {
		t.Error("network should be enabled")
	}
	expectWake(t, env, "enable-me")

	if err := env.Service.SetNetworkEnabled("enable-me", false); err != nil {
		t.Fatalf("disable: %v", err)
	}

	nw, err = env.Service.GetNetwork("enable-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should be disabled")
	}
	expectWake(t, env, "enable-me")
}

func TestSetNetworkEnabled_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.SetNetworkEnabled("ghost", true)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateNetwork_PersistsListenPort(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "portnet")

	port := uint16(51820)
	if err := env.Service.UpdateNetwork(
		"portnet",
		service.NetworkOptions{ListenPort: &port},
	); err != nil {
		t.Fatalf("update: %v", err)
	}

	nw, err := env.Service.GetNetwork("portnet")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.ListenPort != port {
		t.Errorf("listen_port = %d, want %d", nw.ListenPort, port)
	}
	expectWake(t, env, "portnet")
}

func TestUpdateNetwork_RequiresListenPort(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "portnet")

	err := env.Service.UpdateNetwork("portnet", service.NetworkOptions{})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestUninstallNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "to-delete")

	if err := env.Service.UninstallNetwork("to-delete"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	_, err := env.Service.GetNetwork("to-delete")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after uninstall: err = %v, want ErrNotFound", err)
	}
	expectWake(t, env, "to-delete")
}

func TestUninstallNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.UninstallNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// expectWake asserts that the service told the runtime about a change to
// the named network.
func expectWake(
	t *testing.T,
	env *testutil.ServiceEnv,
	network string,
) {
	t.Helper()

	select {
	case name := <-env.Wake:
		if name != network {
			t.Fatalf("wake = %q, want %q", name, network)
		}
	default:
		t.Fatalf("no wake sent for %q", network)
	}
}

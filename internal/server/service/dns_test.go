package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

// seedDNSVisibility seeds the network with a visible pair (frank in ops
// sees api-1 in api through the app:api -> team:ops association), plus
// peers that must never resolve: a hidden peer, a disabled peer, and an
// unconfirmed peer.
func seedDNSVisibility(
	t *testing.T,
	env *testutil.ServiceEnv,
) {
	t.Helper()

	if err := env.Service.CreateCidr("testnet", "api", "10.0.1.0/24"); err != nil {
		t.Fatalf("create api cidr: %v", err)
	}
	if err := env.Service.CreateCidr("testnet", "ops", "10.0.32.0/24"); err != nil {
		t.Fatalf("create ops cidr: %v", err)
	}

	testutil.SeedPeerDB(t, env.Database, "testnet", "api-1", "10.0.1.1/32", "api-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "frank", "10.0.32.5/32", "frank-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "lonely", "10.0.64.5/32", "lonely-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "down", "10.0.64.6/32", "down-key", false, false, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "pending", "10.0.64.7/32", "pending-key", false, true, false)

	for _, group := range []string{"app:api", "team:ops"} {
		if _, err := env.Service.CreateGroup("testnet", group); err != nil {
			t.Fatalf("create group %s: %v", group, err)
		}
	}
	if err := env.Service.AssignCidrGroup("testnet", "api", "app:api"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.AssignCidrGroup("testnet", "ops", "team:ops"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.CreateAssociation("testnet", "app:api", "team:ops"); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDNSHost_Self(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	host, err := env.Service.ResolveDNSHost("testnet", "frank", "frank")
	if err != nil {
		t.Fatalf("resolve self: %v", err)
	}
	if host.Name != "frank" || host.Route != "10.0.32.5/32" {
		t.Fatalf("host = %+v, want frank at 10.0.32.5/32", host)
	}
}

func TestResolveDNSHost_CordServerAlwaysAddressable(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	host, err := env.Service.ResolveDNSHost("testnet", "frank", "cord-server")
	if err != nil {
		t.Fatalf("resolve cord-server: %v", err)
	}
	if host.Name != "cord-server" || host.Route != "10.0.0.1/32" {
		t.Fatalf("host = %+v, want cord-server at 10.0.0.1/32", host)
	}
}

func TestResolveDNSHost_VisiblePeer(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	host, err := env.Service.ResolveDNSHost("testnet", "frank", "api-1")
	if err != nil {
		t.Fatalf("resolve visible peer: %v", err)
	}
	if host.Name != "api-1" || host.Route != "10.0.1.1/32" {
		t.Fatalf("host = %+v, want api-1 at 10.0.1.1/32", host)
	}
}

func TestResolveDNSHost_HiddenPeer(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	_, err := env.Service.ResolveDNSHost("testnet", "frank", "lonely")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("hidden peer error = %v, want ErrNotFound", err)
	}
}

func TestResolveDNSHost_DisabledPeer(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	_, err := env.Service.ResolveDNSHost("testnet", "frank", "down")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("disabled peer error = %v, want ErrNotFound", err)
	}
}

func TestResolveDNSHost_UnconfirmedPeer(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	_, err := env.Service.ResolveDNSHost("testnet", "frank", "pending")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unconfirmed peer error = %v, want ErrNotFound", err)
	}
}

func TestResolveDNSHost_UnknownHost(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	_, err := env.Service.ResolveDNSHost("testnet", "frank", "nobody")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown host error = %v, want ErrNotFound", err)
	}
}

func TestResolveDNSHost_UnknownNetwork(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.ResolveDNSHost("missing", "frank", "frank")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown network error = %v, want ErrNotFound", err)
	}
}

func TestResolveDNSHost_UnknownRequester(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	for _, target := range []string{"frank", "cord-server"} {
		if _, err := env.Service.ResolveDNSHost("testnet", "ghost", target); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("unknown requester resolving %q error = %v, want ErrNotFound", target, err)
		}
	}
}

func TestResolveDNSHost_CaseInsensitive(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	host, err := env.Service.ResolveDNSHost("testnet", "frank", "API-1")
	if err != nil {
		t.Fatalf("resolve mixed-case name: %v", err)
	}
	if host.Name != "api-1" || host.Route != "10.0.1.1/32" {
		t.Fatalf("host = %+v, want stored lowercase api-1 record", host)
	}
}

func TestResolveDNSHost_RejectsNonLabelNames(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedDNSVisibility(t, env)

	for _, name := range []string{"", "a.b", "cord-server.extra"} {
		if _, err := env.Service.ResolveDNSHost("testnet", "frank", name); !errors.Is(err, service.ErrNotFound) {
			t.Fatalf("name %q error = %v, want ErrNotFound", name, err)
		}
	}
}

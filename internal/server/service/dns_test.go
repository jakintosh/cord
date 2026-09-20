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

func TestResolveDNSHost_VisibilityAndNameRules(
	t *testing.T,
) {
	for _, tc := range []struct{ name, network, requester, target, wantName, wantRoute string }{
		{"self", "testnet", "frank", "frank", "frank", "10.0.32.5/32"},
		{"server", "testnet", "frank", "cord-server", "cord-server", "10.0.0.1/32"},
		{"visible", "testnet", "frank", "api-1", "api-1", "10.0.1.1/32"},
		{"case insensitive", "testnet", "frank", "API-1", "api-1", "10.0.1.1/32"},
		{"hidden", "testnet", "frank", "lonely", "", ""},
		{"disabled", "testnet", "frank", "down", "", ""},
		{"unconfirmed", "testnet", "frank", "pending", "", ""},
		{"unknown host", "testnet", "frank", "nobody", "", ""},
		{"unknown network", "missing", "frank", "frank", "", ""},
		{"unknown requester", "testnet", "ghost", "frank", "", ""},
		{"unknown requester to server", "testnet", "ghost", "cord-server", "", ""},
		{"empty name", "testnet", "frank", "", "", ""},
		{"qualified name", "testnet", "frank", "a.b", "", ""},
		{"qualified server", "testnet", "frank", "cord-server.extra", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)
			testutil.SeedNetwork(t, env.Service)
			seedDNSVisibility(t, env)
			host, err := env.Service.ResolveDNSHost(tc.network, tc.requester, tc.target)
			if tc.wantName == "" {
				if !errors.Is(err, service.ErrNotFound) {
					t.Fatalf("resolve: %v, want ErrNotFound", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if host.Name != tc.wantName || host.Route != tc.wantRoute {
				t.Fatalf("resolved host = %+v, want %s at %s", host, tc.wantName, tc.wantRoute)
			}
		})
	}
}

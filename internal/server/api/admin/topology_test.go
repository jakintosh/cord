package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIGetNetworkTopology_SortedFullView(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "later", "10.0.2.0/24")
	env.SeedCIDR(t, "testnet", "earlier", "10.0.1.0/24")
	seedGroup(t, env, "engineering")
	if err := env.Service.AssignCidrGroup("testnet", "earlier", "engineering"); err != nil {
		t.Fatalf("assign group: %v", err)
	}

	result := wire.TestGet[admin.NetworkTopology](
		env.Router,
		"/networks/testnet/topology",
	)
	data := result.ExpectStatusOK(t, http.StatusOK)

	if len(data.Nodes) != 4 {
		t.Fatalf("nodes = %d, want 4", len(data.Nodes))
	}
	if data.Nodes[0].Name != "testnet" {
		t.Fatalf("first node = %q, want root testnet", data.Nodes[0].Name)
	}
	if data.Nodes[1].Name != "cord-server" || data.Nodes[2].Name != "earlier" {
		t.Fatalf("unexpected CIDR order: %#v", data.Nodes[:3])
	}
	if data.Nodes[3].Name != "later" || data.Nodes[3].CIDR != "10.0.2.0/24" {
		t.Fatalf("last node = %+v, want later 10.0.2.0/24", data.Nodes[3])
	}
	root, server, earlier := data.Nodes[0], data.Nodes[1], data.Nodes[2]
	if root.CIDR != "10.0.0.0/16" || root.Terminal || root.DisplayParent != "" {
		t.Fatalf("root = %+v", root)
	}
	if server.CIDR != "10.0.0.1/32" || !server.Terminal || server.PeerName != "cord-server" || server.DisplayParent != "testnet" {
		t.Fatalf("server = %+v", server)
	}
	if earlier.CIDR != "10.0.1.0/24" || earlier.DisplayParent != "testnet" || len(earlier.Groups) != 1 || earlier.Groups[0] != "engineering" {
		t.Fatalf("earlier = %+v", earlier)
	}
}

func TestAPIGetNetworkTopology_NotFound(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	url := "/networks/missing/topology"
	result := wire.TestGet[any](env.Router, url)
	result.ExpectStatusError(t, http.StatusNotFound)
}

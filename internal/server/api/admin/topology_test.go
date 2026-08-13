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
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "later", "10.0.2.0/24")
	env.SeedCIDR(t, "testnet", "earlier", "10.0.1.0/24")

	result := wire.TestGet[admin.NetworkTopology](
		env.Router,
		"/networks/testnet/topology",
	)
	data := result.ExpectStatusOK(t, http.StatusOK)

	if len(data.Nodes) < 3 {
		t.Fatalf("nodes = %d, want at least 3", len(data.Nodes))
	}
	if data.Nodes[0].Name != "testnet" {
		t.Fatalf("first node = %q, want root testnet", data.Nodes[0].Name)
	}
	if data.Nodes[1].Name != "cord-server" || data.Nodes[2].Name != "earlier" {
		t.Fatalf("unexpected CIDR order: %#v", data.Nodes[:3])
	}
}

func TestAPIGetNetworkTopology_NotFound(
	t *testing.T,
) {
	env := testutil.Setup(t)
	result := wire.TestGet[any](env.Router, "/networks/missing/topology")
	result.ExpectStatusError(t, http.StatusNotFound)
}

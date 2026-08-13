package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func TestAPIGetNetworkTopology_FromCache(
	t *testing.T,
) {
	env := testutil.Setup(t)
	testutil.SeedNetworkDirect(t, env.Database, "mynet")
	root, err := topology.CidrFromString("root", "10.42.0.0/16", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Database.ApplyNetworkReconciliation(
		"mynet",
		service.NetworkReconciliation{
			Topology:    topology.View{Nodes: []topology.ViewNode{{Cidr: root}}},
			GeneratedAt: testutil.FixedTime,
			ReceivedAt:  testutil.FixedTime,
			PruneBefore: testutil.FixedTime.Add(-service.EndpointTTL),
		},
	); err != nil {
		t.Fatalf("seed topology: %v", err)
	}

	result := wire.TestGet[api.NetworkTopology](
		env.Router,
		"/networks/mynet/topology",
	)
	data := result.ExpectStatusOK(t, http.StatusOK)
	if len(data.Nodes) != 1 || data.Nodes[0].Name != "root" {
		t.Fatalf("unexpected topology: %#v", data)
	}
}

func TestAPIGetNetworkTopology_UnavailableBeforeSync(
	t *testing.T,
) {
	env := testutil.Setup(t)
	testutil.SeedNetworkDirect(t, env.Database, "mynet")

	result := wire.TestGet[any](env.Router, "/networks/mynet/topology")
	result.ExpectStatusError(t, http.StatusConflict)
}

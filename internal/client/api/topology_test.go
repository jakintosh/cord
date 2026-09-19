package api_test

import (
	"net/http"
	"reflect"
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
	// setup env
	env := testutil.Setup(t)
	testutil.SeedNetworkDirect(t, env.Database, "mynet")
	root, err := topology.CidrFromString("root", "10.42.0.0/16", false)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := topology.CidrFromString("laptop", "10.42.0.9/32", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Database.ApplyNetworkReconciliation(
		"mynet",
		service.NetworkReconciliation{
			Topology: topology.View{
				Nodes: []topology.ViewNode{
					{Cidr: root},
					{Cidr: leaf, DisplayParent: "root", Groups: []string{"staff"}, PeerName: "alice", Subject: true},
				},
				Associations:    []topology.Association{{Group1: "ops", Group2: "staff"}},
				EffectiveGroups: []string{"staff"},
				SubjectPeer:     "alice",
			},
			GeneratedAt: testutil.FixedTime,
			ReceivedAt:  testutil.FixedTime,
			PruneBefore: testutil.FixedTime.Add(-service.EndpointTTL),
		},
	); err != nil {
		t.Fatalf("seed topology: %v", err)
	}

	// send request
	url := "/networks/mynet/topology"
	result := wire.TestGet[api.NetworkTopology](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if !data.GeneratedAt.Equal(testutil.FixedTime) || !data.SyncedAt.Equal(testutil.FixedTime) {
		t.Fatalf("topology timestamps = %v, %v", data.GeneratedAt, data.SyncedAt)
	}
	connected := false
	want := api.NetworkTopology{
		GeneratedAt: data.GeneratedAt,
		SyncedAt:    data.SyncedAt,
		Nodes: []api.TopologyNode{
			{Name: "root", CIDR: "10.42.0.0/16", Groups: []string{}},
			{Name: "laptop", CIDR: "10.42.0.9/32", Terminal: true, DisplayParent: "root", Groups: []string{"staff"}, PeerName: "alice", Subject: true, Connected: &connected},
		},
		Associations:    []api.TopologyAssociation{{Group1: "ops", Group2: "staff"}},
		EffectiveGroups: []string{"staff"},
		SubjectPeer:     "alice",
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("topology = %#v, want %#v", data, want)
	}
}

func TestAPIGetNetworkTopology_UnavailableBeforeSync(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	testutil.SeedNetworkDirect(t, env.Database, "mynet")

	// send request
	url := "/networks/mynet/topology"
	result := wire.TestGet[any](env.Router, url)

	// verify response
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIGetNetworkTopology_MissingNetworkReturnsNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// send request
	url := "/networks/missing/topology"
	result := wire.TestGet[any](env.Router, url)

	// verify response
	result.ExpectStatusError(t, http.StatusNotFound)
}

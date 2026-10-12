package database_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func peerObservation(
	name string,
	key string,
	route string,
	endpoints ...service.PeerEndpoint,
) service.PeerObservation {
	return service.PeerObservation{
		Peer: service.Peer{
			Name:      name,
			PublicKey: key,
			Route:     route,
		},
		Endpoints: endpoints,
	}
}

func cachedView(
	t *testing.T,
	name string,
	cidr string,
) topology.View {
	t.Helper()
	parsed, err := topology.CidrFromString(name, cidr, true)
	if err != nil {
		t.Fatal(err)
	}
	return topology.View{
		SubjectPeer: name,
		Nodes: []topology.ViewNode{
			{Cidr: parsed, PeerName: name, Subject: true},
		},
	}
}

func TestApplyNetworkReconciliation_RejectsInvalidTopologyWithoutChangingState(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	alice := peerObservation("alice", "alice-key", "10.42.0.5/32")
	original := testutil.NetworkReconciliation(alice)
	original.Topology = cachedView(t, "alice", "10.42.0.5/32")
	if err := db.ApplyNetworkReconciliation(
		"testnet",
		original,
	); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}

	cached, err := db.GetNetworkTopology("testnet")
	if err != nil {
		t.Fatalf("get topology: %v", err)
	}
	if cached.View.SubjectPeer != "alice" {
		t.Fatalf("subject = %q, want alice", cached.View.SubjectPeer)
	}

	badNode, err := topology.CidrFromString("bad", "10.42.0.6/32", true)
	if err != nil {
		t.Fatal(err)
	}
	badView := topology.View{
		SubjectPeer: "bad",
		Nodes: []topology.ViewNode{{
			Cidr: badNode, DisplayParent: "missing", PeerName: "bad", Subject: true,
		}},
	}
	bob := peerObservation("bob", "bob-key", "10.42.0.6/32")
	invalid := testutil.NetworkReconciliation(bob)
	invalid.Topology = badView
	if err := db.ApplyNetworkReconciliation(
		"testnet",
		invalid,
	); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].Name != "alice" {
		t.Fatalf("peers changed after rejected snapshot: %#v", peers)
	}
	cached, err = db.GetNetworkTopology("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if cached.View.SubjectPeer != "alice" {
		t.Fatalf("topology changed after rejected snapshot: %#v", cached.View)
	}
}

func TestGetNetworkTopology_Unavailable(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")

	if _, err := db.GetNetworkTopology("testnet"); !errors.Is(err, service.ErrTopologyUnavailable) {
		t.Fatalf("error = %v, want ErrTopologyUnavailable", err)
	}

}

func TestGetNetworkTopology_RoundTripsViewAndTimestamps(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	view := cachedView(t, "alice", "10.42.0.5/32")
	parent, err := topology.CidrFromString("office", "10.42.0.0/24", false)
	if err != nil {
		t.Fatal(err)
	}
	view.Nodes[0].DisplayParent = "office"
	view.Nodes[0].Groups = []string{"staff"}
	view.Nodes = append(view.Nodes, topology.ViewNode{Cidr: parent, Groups: []string{"office"}})
	view.EffectiveGroups = []string{"staff", "office"}
	view.Associations = []topology.Association{{Group1: "staff", Group2: "office"}}
	want, err := topology.NormalizeView(view)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testutil.NetworkReconciliation()
	snapshot.Topology = view
	snapshot.ReceivedAt = testutil.FixedTime.Add(time.Minute)
	if err := db.ApplyNetworkReconciliation("testnet", snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetNetworkTopology("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.View, want) {
		t.Fatalf("view = %#v, want %#v", got.View, want)
	}
	if !got.GeneratedAt.Equal(snapshot.GeneratedAt) || !got.SyncedAt.Equal(snapshot.ReceivedAt) {
		t.Fatalf("cached timestamps = %v / %v", got.GeneratedAt, got.SyncedAt)
	}
}
func TestApplyNetworkReconciliation_InsertsAndListsPeers(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")

	err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("bob", "bob-key", "10.42.0.6/32"),
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	))
	if err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("peers = %d, want 2", len(peers))
	}
	if peers[0].Name != "alice" || peers[1].Name != "bob" {
		t.Fatalf("unexpected order: %q, %q", peers[0].Name, peers[1].Name)
	}
}

func TestApplyNetworkReconciliation_UpdatesAndPrunesPeers(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
		peerObservation("bob", "bob-key", "10.42.0.6/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice-renamed", "alice-key", "10.42.0.9/32"),
	))
	if err != nil {
		t.Fatalf("apply replacement reconciliation: %v", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers = %d, want 1", len(peers))
	}
	if peers[0].Name != "alice-renamed" || peers[0].Route != "10.42.0.9/32" {
		t.Errorf("peer = %#v, want renamed peer and updated route", peers[0])
	}
}

func TestApplyNetworkReconciliation_AllowsDepartedIdentityReplacement(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "old-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "new-key", "10.42.0.5/32"),
	))
	if err != nil {
		t.Fatalf("replace peer identity: %v", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 || peers[0].PublicKey != "new-key" {
		t.Fatalf("peers = %#v, want replacement identity", peers)
	}
}

func TestApplyNetworkReconciliation_EmptyClearsPeers(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation()); err != nil {
		t.Fatalf("apply empty reconciliation: %v", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 0 {
		t.Fatalf("peers = %d, want 0", len(peers))
	}
}

func TestApplyNetworkReconciliation_MissingNetworkLeavesStateUnchanged(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	err := db.ApplyNetworkReconciliation("missing", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	))
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	if _, err := db.ListPeers("missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("peers after rejection: %v, want ErrNotFound", err)
	}
	if _, err := db.GetNetworkTopology("missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("topology after rejection: %v, want ErrNotFound", err)
	}
}

func TestApplyNetworkReconciliation_ConflictsPreserveSnapshot(
	t *testing.T,
) {
	for _, tc := range []struct {
		name  string
		peers []service.PeerObservation
	}{
		{"duplicate name", []service.PeerObservation{
			peerObservation("duplicate", "key-a", "10.42.0.6/32"),
			peerObservation("duplicate", "key-b", "10.42.0.7/32"),
		}},
		{"duplicate public key", []service.PeerObservation{
			peerObservation("alice", "duplicate-key", "10.42.0.6/32"),
			peerObservation("bob", "duplicate-key", "10.42.0.7/32"),
		}},
		{"duplicate route", []service.PeerObservation{
			peerObservation("alice", "key-a", "10.42.0.6/32"),
			peerObservation("bob", "key-b", "10.42.0.6/32"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testutil.SetupDB(t)
			testutil.SeedNetworkDirect(t, db, "testnet")
			original := testutil.NetworkReconciliation(
				peerObservation("existing", "existing-key", "10.42.0.5/32", service.PeerEndpoint{
					Endpoint: "original:51820", ServerObservedAt: testutil.FixedTime,
				}),
			)
			original.Topology = cachedView(t, "existing", "10.42.0.5/32")
			if err := db.ApplyNetworkReconciliation("testnet", original); err != nil {
				t.Fatal(err)
			}
			before, err := db.ListPeers("testnet")
			if err != nil {
				t.Fatal(err)
			}
			endpointsBefore, err := db.ListPeerEndpoints("testnet", "existing-key")
			if err != nil {
				t.Fatal(err)
			}
			topologyBefore, err := db.GetNetworkTopology("testnet")
			if err != nil {
				t.Fatal(err)
			}

			err = db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(tc.peers...))
			if !errors.Is(err, service.ErrConflict) {
				t.Fatalf("error = %v, want ErrConflict", err)
			}
			after, err := db.ListPeers("testnet")
			if err != nil {
				t.Fatal(err)
			}
			endpointsAfter, err := db.ListPeerEndpoints("testnet", "existing-key")
			if err != nil {
				t.Fatal(err)
			}
			topologyAfter, err := db.GetNetworkTopology("testnet")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) || !reflect.DeepEqual(endpointsAfter, endpointsBefore) ||
				!reflect.DeepEqual(topologyAfter, topologyBefore) {
				t.Fatalf("rejected reconciliation changed persisted peers, endpoints, or topology")
			}
		})
	}
}

func TestApplyNetworkReconciliation_MergesEndpointsMonotonically(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	newer := testutil.FixedTime
	older := newer.Add(-time.Hour)
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32", service.PeerEndpoint{
			Endpoint:         "1.2.3.4:51820",
			ServerObservedAt: newer,
		}),
	)); err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}

	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32", service.PeerEndpoint{
			Endpoint:         "1.2.3.4:51820",
			ServerObservedAt: older,
		}),
	)); err != nil {
		t.Fatalf("apply older observation: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(endpoints))
	}
	if !endpoints[0].ServerObservedAt.Equal(newer) {
		t.Errorf("server observed at = %v, want %v", endpoints[0].ServerObservedAt, newer)
	}
}

func TestApplyNetworkReconciliation_RetainsLocalObservationAndAttempt(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	serverObservedAt := testutil.FixedTime.Add(-time.Hour)
	entry := peerObservation(
		"alice",
		"alice-key",
		"10.42.0.5/32",
		service.PeerEndpoint{
			Endpoint:         "1.2.3.4:51820",
			ServerObservedAt: serverObservedAt,
		},
	)
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(entry)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}
	localObservedAt := testutil.FixedTime
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		localObservedAt,
	); err != nil {
		t.Fatalf("record local endpoint: %v", err)
	}
	attemptedAt := testutil.FixedTime.Add(time.Minute)
	if err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		attemptedAt,
	); err != nil {
		t.Fatalf("record endpoint attempt: %v", err)
	}

	entry.Endpoints[0].ServerObservedAt = testutil.FixedTime.Add(time.Hour)
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(entry)); err != nil {
		t.Fatalf("reapply reconciliation: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(endpoints))
	}
	if !endpoints[0].LocalObservedAt.Equal(localObservedAt) {
		t.Errorf("local observed at = %v, want %v", endpoints[0].LocalObservedAt, localObservedAt)
	}
	if !endpoints[0].LastAttemptedAt.Equal(attemptedAt) {
		t.Errorf("last attempted at = %v, want %v", endpoints[0].LastAttemptedAt, attemptedAt)
	}
}

func TestApplyNetworkReconciliation_PrunesOnlyFullyStaleEndpoints(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	stale := testutil.FixedTime.Add(-2 * service.EndpointTTL)
	recentLocal := testutil.FixedTime.Add(-time.Hour)
	seed := testutil.NetworkReconciliation(peerObservation(
		"alice",
		"alice-key",
		"10.42.0.5/32",
		service.PeerEndpoint{Endpoint: "stale:51820", ServerObservedAt: stale},
		service.PeerEndpoint{Endpoint: "local:51820", ServerObservedAt: stale},
	))
	seed.PruneBefore = stale.Add(-time.Hour)
	if err := db.ApplyNetworkReconciliation("testnet", seed); err != nil {
		t.Fatalf("seed endpoints: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"local:51820",
		recentLocal,
	); err != nil {
		t.Fatalf("record local endpoint: %v", err)
	}

	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("apply pruning reconciliation: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(endpoints) != 1 || endpoints[0].Endpoint != "local:51820" {
		t.Fatalf("endpoints after prune = %#v, want locally recent endpoint", endpoints)
	}
}

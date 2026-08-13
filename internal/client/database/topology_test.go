package database_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

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

func networkReconciliation(
	t *testing.T,
	view topology.View,
	peers ...service.PeerObservation,
) service.NetworkReconciliation {
	t.Helper()
	return service.NetworkReconciliation{
		Peers:       peers,
		Topology:    view,
		GeneratedAt: testutil.FixedTime,
		ReceivedAt:  testutil.FixedTime,
		PruneBefore: testutil.FixedTime.Add(-service.EndpointTTL),
	}
}

func TestTopologySchema_UsesTopologyTableName(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	var topologyTables int
	if err := db.Conn.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'topology'`,
	).Scan(&topologyTables); err != nil {
		t.Fatal(err)
	}
	if topologyTables != 1 {
		t.Fatalf("topology tables = %d, want 1", topologyTables)
	}

	var oldNameTables int
	if err := db.Conn.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'network_topology'`,
	).Scan(&oldNameTables); err != nil {
		t.Fatal(err)
	}
	if oldNameTables != 0 {
		t.Fatalf("network_topology tables = %d, want 0", oldNameTables)
	}
}

func TestApplyNetworkReconciliation_ReplacesPeersAndTopologyAtomically(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	alice := peerObservation("alice", "alice-key", "10.42.0.5/32")
	if err := db.ApplyNetworkReconciliation(
		"testnet",
		networkReconciliation(t, cachedView(t, "alice", "10.42.0.5/32"), alice),
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
	if err := db.ApplyNetworkReconciliation(
		"testnet",
		networkReconciliation(t, badView, bob),
	); err == nil {
		t.Fatal("expected invalid topology error")
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

func TestGetNetworkTopology_UnavailableAndCascade(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")

	if _, err := db.GetNetworkTopology("testnet"); !errors.Is(err, service.ErrTopologyUnavailable) {
		t.Fatalf("error = %v, want ErrTopologyUnavailable", err)
	}
	if err := db.ApplyNetworkReconciliation(
		"testnet",
		networkReconciliation(t, cachedView(t, "self", "10.42.0.5/32")),
	); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteNetworkState("testnet"); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM topology`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("topology rows after uninstall = %d, want 0", count)
	}
}

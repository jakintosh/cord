package peer_test

import (
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIVisibleSnapshot_ProjectsTopologyAndCollapsesWitnesses(
	t *testing.T,
) {
	// Setup visible peers and three witnesses; two observed the same endpoint.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "bob", "10.0.0.6/32", "bob-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "carol", "10.0.0.7/32", "carol-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "dave", "10.0.0.8/32", "dave-key", false, true, true)
	if _, err := env.Service.CreateGroup("testnet", "friends"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "bob"} {
		if err := env.Service.AssignCidrGroup("testnet", name, "friends"); err != nil {
			t.Fatal(err)
		}
	}
	if err := env.Service.CreateAssociation("testnet", "friends", "friends"); err != nil {
		t.Fatal(err)
	}
	older := testutil.FixedTime.Add(-time.Minute)
	newest := testutil.FixedTime.Add(-time.Second)
	if err := env.Database.InsertEndpointSightings("testnet", []service.EndpointSighting{
		{WitnessKey: "alice-key", PeerKey: "bob-key", Endpoint: "2.3.4.5:51820", Timestamp: older},
		{WitnessKey: "carol-key", PeerKey: "bob-key", Endpoint: "2.3.4.5:51820", Timestamp: newest},
		{WitnessKey: "dave-key", PeerKey: "bob-key", Endpoint: "1.2.3.4:51820", Timestamp: older},
	}); err != nil {
		t.Fatal(err)
	}
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// Read the public DTO through the authenticated route.
	url := "/snapshot"
	result := wire.TestGet[protocol.VisibleNetworkSnapshot](router, url)

	// Verify snapshot time and projected topology.
	data := result.ExpectOK(t)
	if !data.GeneratedAt.Equal(testutil.FixedTime) || data.Topology.SubjectPeer != "alice" {
		t.Fatalf("snapshot = %+v", data)
	}
	foundSubject := false
	foundBob := false
	for _, node := range data.Topology.Nodes {
		if node.PeerName == "alice" {
			foundSubject = node.Subject && node.Terminal && node.CIDR == "10.0.0.5/32"
		}
		if node.PeerName == "bob" {
			foundBob = node.Terminal && node.CIDR == "10.0.0.6/32" && !node.Subject
		}
	}
	if !foundSubject || !foundBob {
		t.Fatalf("topology = %+v", data.Topology)
	}
	var bob *protocol.VisiblePeer
	for i := range data.Peers {
		if data.Peers[i].Name == "bob" {
			bob = &data.Peers[i]
		}
		if data.Peers[i].Name == "carol" || data.Peers[i].Name == "dave" {
			t.Fatalf("unassociated witness leaked: %+v", data.Peers[i])
		}
	}
	if bob == nil || bob.PublicKey != "bob-key" || bob.Route != "10.0.0.6/32" {
		t.Fatalf("peers = %+v", data.Peers)
	}
	if len(bob.Endpoints) != 2 || bob.Endpoints[0].Endpoint != "1.2.3.4:51820" || !bob.Endpoints[0].Timestamp.Equal(older) || bob.Endpoints[1].Endpoint != "2.3.4.5:51820" || !bob.Endpoints[1].Timestamp.Equal(newest) {
		t.Fatalf("collapsed endpoints = %+v", bob.Endpoints)
	}
}

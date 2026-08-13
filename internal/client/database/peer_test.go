package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func reconciliation(
	peers ...service.PeerObservation,
) service.PeerReconciliation {
	return service.PeerReconciliation{
		Peers:       peers,
		PruneBefore: testutil.FixedTime.Add(-service.EndpointTTL),
	}
}

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

func TestApplyPeerReconciliation_InsertsAndListsPeers(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")

	err := db.ApplyPeerReconciliation("testnet", reconciliation(
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

func TestApplyPeerReconciliation_UpdatesAndPrunesPeers(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
		peerObservation("bob", "bob-key", "10.42.0.6/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	err := db.ApplyPeerReconciliation("testnet", reconciliation(
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

func TestApplyPeerReconciliation_AllowsDepartedIdentityReplacement(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "old-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	err := db.ApplyPeerReconciliation("testnet", reconciliation(
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

func TestApplyPeerReconciliation_EmptyClearsPeers(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	if err := db.ApplyPeerReconciliation("testnet", reconciliation()); err != nil {
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

func TestApplyPeerReconciliation_MissingNetworkLeavesStateUnchanged(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.ApplyPeerReconciliation("missing", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	))
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	var peers int
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM peer`).Scan(&peers); err != nil {
		t.Fatalf("count peers: %v", err)
	}
	if peers != 0 {
		t.Errorf("peers after rejection = %d, want 0", peers)
	}
}

func TestApplyPeerReconciliation_IdentityConflictRollsBack(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("existing", "existing-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("duplicate", "key-a", "10.42.0.6/32"),
		peerObservation("duplicate", "key-b", "10.42.0.7/32"),
	))
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers after rejection: %v", err)
	}
	if len(peers) != 1 || peers[0].Name != "existing" {
		t.Fatalf("peers after rejection = %#v, want original peer", peers)
	}
}

func TestApplyPeerReconciliation_DuplicatePublicKeyLeavesStateUnchanged(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("existing", "existing-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("seed reconciliation: %v", err)
	}

	err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "duplicate-key", "10.42.0.6/32"),
		peerObservation("bob", "duplicate-key", "10.42.0.7/32"),
	))
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers after rejection: %v", err)
	}
	if len(peers) != 1 || peers[0].Name != "existing" {
		t.Fatalf("peers after rejection = %#v, want original peer", peers)
	}
}

func TestApplyPeerReconciliation_MergesEndpointsMonotonically(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	newer := testutil.FixedTime
	older := newer.Add(-time.Hour)
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32", service.PeerEndpoint{
			Endpoint:         "1.2.3.4:51820",
			ServerObservedAt: newer,
		}),
	)); err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}

	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
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

func TestApplyPeerReconciliation_RetainsLocalObservationAndAttempt(t *testing.T) {
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
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(entry)); err != nil {
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
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(entry)); err != nil {
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

func TestApplyPeerReconciliation_PrunesOnlyFullyStaleEndpoints(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	stale := testutil.FixedTime.Add(-2 * service.EndpointTTL)
	recentLocal := testutil.FixedTime.Add(-time.Hour)
	if err := db.ApplyPeerReconciliation("testnet", service.PeerReconciliation{
		Peers: []service.PeerObservation{
			peerObservation(
				"alice",
				"alice-key",
				"10.42.0.5/32",
				service.PeerEndpoint{Endpoint: "stale:51820", ServerObservedAt: stale},
				service.PeerEndpoint{Endpoint: "local:51820", ServerObservedAt: stale},
			),
		},
		PruneBefore: stale.Add(-time.Hour),
	}); err != nil {
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

	if err := db.ApplyPeerReconciliation("testnet", service.PeerReconciliation{
		Peers: []service.PeerObservation{
			peerObservation("alice", "alice-key", "10.42.0.5/32"),
		},
		PruneBefore: testutil.FixedTime.Add(-service.EndpointTTL),
	}); err != nil {
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

func TestListPeers_MissingNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.ListPeers("missing")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListPeers_PrefersMostRecentLocalEndpoint(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation(
			"alice",
			"alice-key",
			"10.42.0.5/32",
			service.PeerEndpoint{
				Endpoint:         "server:51820",
				ServerObservedAt: testutil.FixedTime,
			},
		),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"local:51820",
		testutil.FixedTime.Add(-time.Hour),
	); err != nil {
		t.Fatalf("record local endpoint: %v", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers = %d, want 1", len(peers))
	}
	if peers[0].Endpoint != "local:51820" {
		t.Errorf("endpoint = %q, want local endpoint", peers[0].Endpoint)
	}
}

func TestRecordLocalEndpoint_MissingPeer(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")

	err := db.RecordLocalEndpoint(
		"testnet",
		"missing-key",
		"1.2.3.4:51820",
		testutil.FixedTime,
	)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecordLocalEndpoint_DoesNotRegress(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	newer := testutil.FixedTime
	older := newer.Add(-time.Hour)
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		newer,
	); err != nil {
		t.Fatalf("record newer endpoint: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		older,
	); err != nil {
		t.Fatalf("record older endpoint: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if !endpoints[0].LocalObservedAt.Equal(newer) {
		t.Errorf("local observed at = %v, want %v", endpoints[0].LocalObservedAt, newer)
	}
}

func TestRecordEndpointAttempt_MissingEndpoint(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}

	err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"missing:51820",
		testutil.FixedTime,
	)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecordEndpointAttempt_DoesNotRegress(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation(
			"alice",
			"alice-key",
			"10.42.0.5/32",
			service.PeerEndpoint{
				Endpoint:         "1.2.3.4:51820",
				ServerObservedAt: testutil.FixedTime,
			},
		),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	newer := testutil.FixedTime
	older := newer.Add(-time.Hour)
	if err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		newer,
	); err != nil {
		t.Fatalf("record newer attempt: %v", err)
	}
	if err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		older,
	); err != nil {
		t.Fatalf("record older attempt: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if !endpoints[0].LastAttemptedAt.Equal(newer) {
		t.Errorf("last attempted at = %v, want %v", endpoints[0].LastAttemptedAt, newer)
	}
}

func TestListLocalEndpointsSince(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyPeerReconciliation("testnet", reconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
		peerObservation("bob", "bob-key", "10.42.0.6/32"),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		testutil.FixedTime,
	); err != nil {
		t.Fatalf("record alice endpoint: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"bob-key",
		"5.6.7.8:51820",
		testutil.FixedTime.Add(-time.Hour),
	); err != nil {
		t.Fatalf("record bob endpoint: %v", err)
	}

	sightings, err := db.ListLocalEndpointsSince(
		"testnet",
		testutil.FixedTime.Add(-30*time.Minute),
	)
	if err != nil {
		t.Fatalf("list local endpoints: %v", err)
	}
	if len(sightings) != 1 {
		t.Fatalf("sightings = %d, want 1", len(sightings))
	}
	if sightings[0].PeerKey != "alice-key" ||
		sightings[0].Endpoint != "1.2.3.4:51820" {
		t.Errorf("sighting = %#v, want alice endpoint", sightings[0])
	}
}

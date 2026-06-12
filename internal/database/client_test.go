package database_test

import (
	"os"
	"path"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/client"
	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// setupTestClientDB creates a new in-memory client database for testing
func setupTestClientDB(t *testing.T) *database.ClientDB {
	t.Helper()
	store, err := database.OpenClient(database.Options{
		Name: "test-network",
		Dir:  ":memory:",
	})
	if err != nil {
		t.Fatalf("failed to open test client database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func publicPeer(name, key, cidr string) server.PublicPeer {
	return server.PublicPeer{
		Name:      name,
		PublicKey: key,
		Cidr:      cidr,
	}
}

func publicPeerWithEndpoint(name, key, cidr, endpoint string, when int64) server.PublicPeer {
	peer := publicPeer(name, key, cidr)
	peer.Endpoints = []server.EndpointWitness{
		{Endpoint: endpoint, Timestamp: when},
	}
	return peer
}

// assertLocalPeers verifies the stored peer names, in order.
func assertLocalPeers(t *testing.T, store *database.ClientDB, names ...string) {
	t.Helper()
	peers, err := store.ListPeers()
	if err != nil {
		t.Fatalf("failed to list peers: %v", err)
	}
	if len(peers) != len(names) {
		t.Fatalf("got %d peers, want %d", len(peers), len(names))
	}
	for i, name := range names {
		if peers[i].Name != name {
			t.Fatalf("peer[%d].Name = %q, want %q", i, peers[i].Name, name)
		}
	}
}

// localPeerByKey fetches one stored peer by public key.
func localPeerByKey(t *testing.T, store *database.ClientDB, key string) *client.LocalPeer {
	t.Helper()
	peers, err := store.ListPeers()
	if err != nil {
		t.Fatalf("failed to list peers: %v", err)
	}
	for _, peer := range peers {
		if peer.PublicKey == key {
			return &peer
		}
	}
	return nil
}

func TestReconcilePeers_InsertsAndOrdersByName(t *testing.T) {
	store := setupTestClientDB(t)

	err := store.ReconcilePeers([]server.PublicPeer{
		publicPeer("zeta", "key-z", "10.0.0.3/32"),
		publicPeer("alpha", "key-a", "10.0.0.2/32"),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	assertLocalPeers(t, store, "alpha", "zeta")
}

func TestReconcilePeers_KeepsFresherLocalEndpoint(t *testing.T) {
	store := setupTestClientDB(t)

	err := store.ReconcilePeers([]server.PublicPeer{
		publicPeer("alpha", "key-a", "10.0.0.2/32"),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	// a locally observed endpoint, newer than what the server reports
	if err := store.UpdateEndpoint("key-a", "198.51.100.9:51820", 200); err != nil {
		t.Fatalf("failed to update endpoint: %v", err)
	}

	err = store.ReconcilePeers([]server.PublicPeer{
		publicPeerWithEndpoint("alpha", "key-a", "10.0.0.2/32", "203.0.113.5:51820", 100),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	peer := localPeerByKey(t, store, "key-a")
	if peer == nil {
		t.Fatalf("peer 'alpha' missing after reconcile")
	}
	if peer.Endpoint != "198.51.100.9:51820" {
		t.Fatalf("endpoint = %q, want fresher local endpoint kept", peer.Endpoint)
	}
	if peer.EndpointTime != 200 {
		t.Fatalf("endpoint_time = %d, want 200", peer.EndpointTime)
	}
}

func TestReconcilePeers_AcceptsNewerServerEndpoint(t *testing.T) {
	store := setupTestClientDB(t)

	err := store.ReconcilePeers([]server.PublicPeer{
		publicPeerWithEndpoint("alpha", "key-a", "10.0.0.2/32", "203.0.113.5:51820", 100),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	err = store.ReconcilePeers([]server.PublicPeer{
		publicPeerWithEndpoint("alpha", "key-a", "10.0.0.2/32", "203.0.113.6:51820", 300),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	peer := localPeerByKey(t, store, "key-a")
	if peer == nil {
		t.Fatalf("peer 'alpha' missing after reconcile")
	}
	if peer.Endpoint != "203.0.113.6:51820" {
		t.Fatalf("endpoint = %q, want newer server endpoint applied", peer.Endpoint)
	}
}

func TestReconcilePeers_PrunesDepartedPeers(t *testing.T) {
	store := setupTestClientDB(t)

	err := store.ReconcilePeers([]server.PublicPeer{
		publicPeer("alpha", "key-a", "10.0.0.2/32"),
		publicPeer("beta", "key-b", "10.0.0.3/32"),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	err = store.ReconcilePeers([]server.PublicPeer{
		publicPeer("alpha", "key-a", "10.0.0.2/32"),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	assertLocalPeers(t, store, "alpha")
}

func TestReconcilePeers_EmptyListClearsPeers(t *testing.T) {
	store := setupTestClientDB(t)

	err := store.ReconcilePeers([]server.PublicPeer{
		publicPeer("alpha", "key-a", "10.0.0.2/32"),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	if err := store.ReconcilePeers(nil); err != nil {
		t.Fatalf("failed to reconcile empty peer list: %v", err)
	}

	assertLocalPeers(t, store)
}

func TestUpdateEndpoint_Roundtrips(t *testing.T) {
	store := setupTestClientDB(t)

	err := store.ReconcilePeers([]server.PublicPeer{
		publicPeer("alpha", "key-a", "10.0.0.2/32"),
	})
	if err != nil {
		t.Fatalf("failed to reconcile peers: %v", err)
	}

	if err := store.UpdateEndpoint("key-a", "198.51.100.9:51820", 42); err != nil {
		t.Fatalf("failed to update endpoint: %v", err)
	}

	peer := localPeerByKey(t, store, "key-a")
	if peer == nil {
		t.Fatalf("peer 'alpha' missing")
	}
	if peer.Endpoint != "198.51.100.9:51820" || peer.EndpointTime != 42 {
		t.Fatalf("endpoint = %q (%d), want recorded sighting", peer.Endpoint, peer.EndpointTime)
	}
}

func TestUpdateEndpoint_UnknownKeyIsNoOp(t *testing.T) {
	store := setupTestClientDB(t)

	if err := store.UpdateEndpoint("missing-key", "198.51.100.9:51820", 42); err != nil {
		t.Fatalf("expected silent no-op for unknown key, got: %v", err)
	}

	assertLocalPeers(t, store)
}

func TestClientDelete_RemovesDatabaseFile(t *testing.T) {
	dir := t.TempDir()
	store, err := database.OpenClient(database.Options{Name: "delete-test", Dir: dir})
	if err != nil {
		t.Fatalf("failed to open client database: %v", err)
	}

	dbFile := path.Join(dir, "delete-test.db")
	if _, err := os.Stat(dbFile); err != nil {
		t.Fatalf("expected database file to exist: %v", err)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("failed to delete database: %v", err)
	}
	if _, err := os.Stat(dbFile); !os.IsNotExist(err) {
		t.Fatalf("expected database file to be removed, got: %v", err)
	}
}

func TestClientDelete_ToleratesMissingFile(t *testing.T) {
	dir := t.TempDir()
	store, err := database.OpenClient(database.Options{Name: "delete-test", Dir: dir})
	if err != nil {
		t.Fatalf("failed to open client database: %v", err)
	}

	if err := os.Remove(path.Join(dir, "delete-test.db")); err != nil {
		t.Fatalf("failed to remove database file: %v", err)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("expected delete to tolerate a missing file, got: %v", err)
	}
}

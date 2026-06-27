package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func seedNetwork(t *testing.T, db *database.DB) {
	t.Helper()
	now := time.Now()
	if err := db.InsertNetwork(&service.Network{
		Name:           "testnet",
		PrivateKey:     "priv-test",
		PublicKey:      "pub-test",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "server-pub-key",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

func TestSetPeers_Insert(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	peers := []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
		{Name: "bob", PublicKey: "bob-key", Cidr: "10.42.1.6/32"},
		{Name: "charlie", PublicKey: "charlie-key", Cidr: "10.42.1.7/32"},
	}

	if err := db.SetPeers("testnet", peers); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(got))
	}
	for i, p := range got {
		if p.Name != peers[i].Name {
			t.Errorf("peer[%d] name = %q, want %q", i, p.Name, peers[i].Name)
		}
		if p.PublicKey != peers[i].PublicKey {
			t.Errorf("peer[%d] public_key = %q, want %q", i, p.PublicKey, peers[i].PublicKey)
		}
		if p.Cidr != peers[i].Cidr {
			t.Errorf("peer[%d] cidr = %q, want %q", i, p.Cidr, peers[i].Cidr)
		}
	}
}

func TestSetPeers_Upsert(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "original", PublicKey: "peer-key", Cidr: "10.42.1.10/32"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "renamed", PublicKey: "peer-key", Cidr: "10.42.1.20/32"},
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(got))
	}
	if got[0].Name != "renamed" {
		t.Errorf("name = %q, want renamed", got[0].Name)
	}
	if got[0].Cidr != "10.42.1.20/32" {
		t.Errorf("cidr = %q, want 10.42.1.20/32", got[0].Cidr)
	}
	if got[0].PublicKey != "peer-key" {
		t.Errorf("public_key = %q, want peer-key", got[0].PublicKey)
	}
}

func TestSetPeers_Prune(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "keep", PublicKey: "keep-key", Cidr: "10.42.1.1/32"},
		{Name: "remove", PublicKey: "remove-key", Cidr: "10.42.1.2/32"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "keep", PublicKey: "keep-key", Cidr: "10.42.1.1/32"},
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(got))
	}
	if got[0].Name != "keep" {
		t.Errorf("remaining peer = %q, want keep", got[0].Name)
	}
}

func TestSetPeers_ClearAll(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.1/32"},
		{Name: "bob", PublicKey: "bob-key", Cidr: "10.42.1.2/32"},
	}); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}

	if err := db.SetPeers("testnet", nil); err != nil {
		t.Fatalf("clear reconcile: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(got))
	}
}

func TestSetPeers_NetworkIsolation(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	now := time.Now()
	if err := db.InsertNetwork(&service.Network{
		Name:           "othernet",
		PrivateKey:     "priv-other",
		PublicKey:      "pub-other",
		AssignedCidr:   "10.43.0.5/16",
		ServerPubkey:   "srv-key-2",
		ServerEndpoint: "5.6.7.8:51820",
		ServerApiAddr:  "10.43.0.1:8443",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("insert other network: %v", err)
	}

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("reconcile testnet: %v", err)
	}

	gotTest, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list testnet peers: %v", err)
	}
	if len(gotTest) != 1 {
		t.Fatalf("expected 1 peer in testnet, got %d", len(gotTest))
	}

	gotOther, err := db.ListPeers("othernet")
	if err != nil {
		t.Fatalf("list othernet peers: %v", err)
	}
	if len(gotOther) != 0 {
		t.Fatalf("expected 0 peers in othernet, got %d", len(gotOther))
	}
}

func TestListPeers_Ordered(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "ccc", PublicKey: "ccc-key", Cidr: "10.42.1.3/32"},
		{Name: "aaa", PublicKey: "aaa-key", Cidr: "10.42.1.1/32"},
		{Name: "bbb", PublicKey: "bbb-key", Cidr: "10.42.1.2/32"},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(got))
	}
	if got[0].Name != "aaa" || got[1].Name != "bbb" || got[2].Name != "ccc" {
		t.Errorf("unexpected order: %v, %v, %v", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestListPeers_EmptyNetwork(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(got))
	}
}

func TestListPeers_WithEndpoint(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("set peers: %v", err)
	}

	// Add endpoints.
	if err := db.SetPeerEndpoints("testnet", "alice-key", []service.PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", ServerObservedAt: 100},
		{Endpoint: "2.2.2.2:51820", ServerObservedAt: 200},
	}); err != nil {
		t.Fatalf("set endpoints: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(got))
	}
	// Should pick the endpoint with the highest server_observed_at.
	if got[0].Endpoint != "2.2.2.2:51820" {
		t.Errorf("endpoint = %q, want 2.2.2.2:51820", got[0].Endpoint)
	}
}

func TestSetPeerEndpoints_Upsert(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("set peers: %v", err)
	}

	// Initial endpoints.
	if err := db.SetPeerEndpoints("testnet", "alice-key", []service.PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", ServerObservedAt: 100},
		{Endpoint: "2.2.2.2:51820", ServerObservedAt: 200},
	}); err != nil {
		t.Fatalf("first set endpoints: %v", err)
	}

	// Update with new data — keep 2.2.2.2, add 3.3.3.3, drop 1.1.1.1.
	if err := db.SetPeerEndpoints("testnet", "alice-key", []service.PeerEndpoint{
		{Endpoint: "2.2.2.2:51820", ServerObservedAt: 300},
		{Endpoint: "3.3.3.3:51820", ServerObservedAt: 250},
	}); err != nil {
		t.Fatalf("second set endpoints: %v", err)
	}

	eps, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(eps))
	}
	if eps[0].Endpoint != "2.2.2.2:51820" {
		t.Errorf("endpoint[0] = %q, want 2.2.2.2:51820", eps[0].Endpoint)
	}
	if eps[0].ServerObservedAt != 300 {
		t.Errorf("server_observed_at[0] = %d, want 300", eps[0].ServerObservedAt)
	}
}

func TestUpdatePeerEndpointLocal(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("set peers: %v", err)
	}

	if err := db.SetPeerEndpoints("testnet", "alice-key", []service.PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", ServerObservedAt: 100},
	}); err != nil {
		t.Fatalf("set endpoints: %v", err)
	}

	// Record local observation.
	if err := db.UpdatePeerEndpointLocal("testnet", "alice-key", "1.1.1.1:51820", 500); err != nil {
		t.Fatalf("update local: %v", err)
	}

	eps, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(eps))
	}
	if eps[0].LocalObservedAt != 500 {
		t.Errorf("local_observed_at = %d, want 500", eps[0].LocalObservedAt)
	}

	// ListPeers should now prefer this endpoint (local_observed_at breaks ties).
	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if got[0].Endpoint != "1.1.1.1:51820" {
		t.Errorf("endpoint = %q, want 1.1.1.1:51820", got[0].Endpoint)
	}
}

func TestUpdatePeerEndpointLocal_Nonexistent(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	// Should be a no-op, not an error.
	err := db.UpdatePeerEndpointLocal("testnet", "nobody-key", "1.2.3.4:51820", 100)
	if err != nil {
		t.Fatalf("update nonexistent should not error: %v", err)
	}
}

func TestListPeerEndpoints_Empty(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("set peers: %v", err)
	}

	eps, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 endpoints, got %d", len(eps))
	}
}

func TestSetPeerEndpoints_ClearAll(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("set peers: %v", err)
	}

	if err := db.SetPeerEndpoints("testnet", "alice-key", []service.PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", ServerObservedAt: 100},
	}); err != nil {
		t.Fatalf("set endpoints: %v", err)
	}

	// Clear all endpoints.
	if err := db.SetPeerEndpoints("testnet", "alice-key", nil); err != nil {
		t.Fatalf("clear endpoints: %v", err)
	}

	eps, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 endpoints after clear, got %d", len(eps))
	}
}

func TestSetPeerEndpoints_CascadeDelete(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.SetPeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("set peers: %v", err)
	}

	if err := db.SetPeerEndpoints("testnet", "alice-key", []service.PeerEndpoint{
		{Endpoint: "1.1.1.1:51820", ServerObservedAt: 100},
	}); err != nil {
		t.Fatalf("set endpoints: %v", err)
	}

	// Remove the peer — endpoints should cascade delete.
	if err := db.SetPeers("testnet", nil); err != nil {
		t.Fatalf("clear peers: %v", err)
	}

	eps, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 endpoints after peer delete, got %d", len(eps))
	}
}

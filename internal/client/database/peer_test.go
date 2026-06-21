package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/testutil"
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

func TestReconcilePeers_Insert(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	peers := []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
		{Name: "bob", PublicKey: "bob-key", Cidr: "10.42.1.6/32"},
		{Name: "charlie", PublicKey: "charlie-key", Cidr: "10.42.1.7/32"},
	}

	if err := db.ReconcilePeers("testnet", peers); err != nil {
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

func TestReconcilePeers_Upsert(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{Name: "original", PublicKey: "peer-key", Cidr: "10.42.1.10/32"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := db.ReconcilePeers("testnet", []service.Peer{
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

func TestReconcilePeers_Prune(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{Name: "keep", PublicKey: "keep-key", Cidr: "10.42.1.1/32"},
		{Name: "remove", PublicKey: "remove-key", Cidr: "10.42.1.2/32"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := db.ReconcilePeers("testnet", []service.Peer{
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

func TestReconcilePeers_ClearAll(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.1/32"},
		{Name: "bob", PublicKey: "bob-key", Cidr: "10.42.1.2/32"},
	}); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}

	if err := db.ReconcilePeers("testnet", nil); err != nil {
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

func TestReconcilePeers_EndpointPreserved(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{
			Name:         "alice",
			PublicKey:    "alice-key",
			Cidr:         "10.42.1.5/32",
			Endpoint:     "2.3.4.5:51820",
			EndpointTime: 200,
		},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{
			Name:         "alice",
			PublicKey:    "alice-key",
			Cidr:         "10.42.1.5/32",
			Endpoint:     "5.6.7.8:51820",
			EndpointTime: 100,
		},
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
	if got[0].Endpoint != "2.3.4.5:51820" {
		t.Errorf("endpoint = %q, want 2.3.4.5:51820 (locally fresher, should be preserved)", got[0].Endpoint)
	}
	if got[0].EndpointTime != 200 {
		t.Errorf("endpoint_time = %d, want 200", got[0].EndpointTime)
	}
}

func TestReconcilePeers_EndpointOverwritten(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{
			Name:         "bob",
			PublicKey:    "bob-key",
			Cidr:         "10.42.1.6/32",
			Endpoint:     "2.3.4.5:51820",
			EndpointTime: 50,
		},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{
			Name:         "bob",
			PublicKey:    "bob-key",
			Cidr:         "10.42.1.6/32",
			Endpoint:     "9.10.11.12:51820",
			EndpointTime: 200,
		},
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if got[0].Endpoint != "9.10.11.12:51820" {
		t.Errorf("endpoint = %q, want 9.10.11.12:51820 (server report is fresher)", got[0].Endpoint)
	}
	if got[0].EndpointTime != 200 {
		t.Errorf("endpoint_time = %d, want 200", got[0].EndpointTime)
	}
}

func TestReconcilePeers_NetworkIsolation(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
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

	if err := db.ReconcilePeers("testnet", []service.Peer{
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
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
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
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(got))
	}
}

func TestUpdatePeerEndpoint(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	if err := db.ReconcilePeers("testnet", []service.Peer{
		{Name: "alice", PublicKey: "alice-key", Cidr: "10.42.1.5/32"},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if err := db.UpdatePeerEndpoint("testnet", "alice-key", "5.6.7.8:51820", 1234567890); err != nil {
		t.Fatalf("update endpoint: %v", err)
	}

	got, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(got))
	}
	if got[0].Endpoint != "5.6.7.8:51820" {
		t.Errorf("endpoint = %q, want 5.6.7.8:51820", got[0].Endpoint)
	}
	if got[0].EndpointTime != 1234567890 {
		t.Errorf("endpoint_time = %d, want 1234567890", got[0].EndpointTime)
	}
}

func TestUpdatePeerEndpoint_Nonexistent(t *testing.T) {
	db := testutil.SetupClientTestDB(t)
	seedNetwork(t, db)

	err := db.UpdatePeerEndpoint("testnet", "nobody-key", "1.2.3.4:51820", 100)
	if err != nil {
		t.Fatalf("update nonexistent peer endpoint should not error: %v", err)
	}
}

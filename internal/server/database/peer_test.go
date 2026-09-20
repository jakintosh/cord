package database_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestUpdatePeer_PreservesUnspecifiedFieldsAndAppliesFalse(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)
	testutil.SeedPeerDB(t, db, "testnet", "alice", "10.0.5.1/32", "alice-key", true, true, true)
	before, err := db.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdatePeer("testnet", "alice", service.PeerDiff{}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("empty update changed peer: %+v -> %+v", before, got)
	}
	no := false
	if _, err := db.UpdatePeer("testnet", "alice", service.PeerDiff{Admin: &no, Enabled: &no}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}
	before.Admin, before.Enabled = false, false
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("false update = %+v, want %+v", got, before)
	}
	testutil.SeedPeerDB(t, db, "testnet", "bob", "10.0.5.2/32", "bob-key", false, true, true)
	name, yes := "bob", true
	if _, err := db.UpdatePeer("testnet", "alice", service.PeerDiff{Name: &name, Admin: &yes}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("conflicting rename = %v, want ErrConflict", err)
	}
	got, err = db.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected update changed peer: %+v", got)
	}
}

func TestInsertPeer_RoundTrip(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	testutil.SeedPeerDB(t, db, "testnet", "alice", "10.0.5.1/32", "alice-pub-key", true, true, true)

	got, err := db.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}

	if got.Name != "alice" {
		t.Errorf("name = %q, want %q", got.Name, "alice")
	}
	if got.PublicKey != "alice-pub-key" {
		t.Errorf("public_key = %q, want %q", got.PublicKey, "alice-pub-key")
	}
	if got.Route != "10.0.5.1/32" {
		t.Errorf("cidr = %q, want %q", got.Route, "10.0.5.1/32")
	}
	if got.Admin != true {
		t.Errorf("admin = %v, want %v", got.Admin, true)
	}
	if got.Enabled != true {
		t.Errorf("enabled = %v, want %v", got.Enabled, true)
	}
	if got.Confirmed != true {
		t.Errorf("confirmed = %v, want %v", got.Confirmed, true)
	}
}

func TestGetPeer_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	_, err := db.GetPeer("testnet", "nobody")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected error for nonexistent peer: %v", err)
	}
}

func TestGetPeer_WrongNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	testutil.SeedPeerDB(t, db, "testnet", "alice", "10.0.5.1/32", "alice-key", false, false, false)

	_, err := db.GetPeer("othernet", "alice")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected error for wrong network: %v", err)
	}
}

func TestListPeers_SortedByName(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	testutil.SeedPeerDB(t, db, "testnet", "ddd", "10.0.5.10/32", "ddd-key", false, false, false)
	testutil.SeedPeerDB(t, db, "testnet", "aaa", "10.0.5.11/32", "aaa-key", false, false, false)

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(peers))
	}
	if peers[0].Name != "aaa" || peers[1].Name != "cord-server" || peers[2].Name != "ddd" {
		t.Errorf("unexpected order: %v, %v, %v", peers[0].Name, peers[1].Name, peers[2].Name)
	}
}

func TestListPeers_EmptyNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
}

func TestInsertPeer_DuplicateName(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.CreateCidr("testnet", &service.Cidr{
		Name: "dup", Cidr: "10.0.5.20/32",
	}); err != nil {
		t.Fatalf("first insert cidr: %v", err)
	}
	peer := &service.Peer{
		Name:      "dup",
		CidrName:  "dup",
		PublicKey: "key-a",
	}
	if err := db.InsertPeer("testnet", peer); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	if err := db.CreateCidr("testnet", &service.Cidr{
		Name: "dup2", Cidr: "10.0.5.21/32",
	}); err != nil {
		t.Fatalf("second insert cidr: %v", err)
	}
	peer.PublicKey = "key-b"
	peer.CidrName = "dup2"
	err := db.InsertPeer("testnet", peer)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("expected error for duplicate peer name: %v", err)
	}
}

func TestInsertPeer_SameNameDifferentNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	now := time.Now()
	if err := db.BootstrapNetwork(
		&service.Network{
			Name:       "net2",
			PrivateKey: "priv2",
			PublicKey:  "pub2",
			ExternalIP: "1.1.1.2",
			Main: service.Plane{
				Name:          "net2",
				Cidr:          "172.16.0.0/16",
				WireguardPort: 51822,
				ApiPort:       8081,
			},
			Invite: service.Plane{
				Name:          "net2-i",
				Cidr:          "172.17.0.0/24",
				WireguardPort: 51823,
				ApiPort:       8082,
			},
			CreatedAt: now,
		},
		&service.Cidr{
			Name:   "net2",
			Cidr:   "172.16.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server",
			Cidr:     "172.16.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server",
			Route:     "172.16.0.1/32",
			PublicKey: "pub2",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("insert net2: %v", err)
	}

	testutil.SeedPeerDB(t, db, "testnet", "shared", "10.0.5.40/32", "key-a", false, false, false)
	testutil.SeedPeerDB(t, db, "net2", "shared", "172.16.5.40/32", "key-b", false, false, false)

	got, err := db.GetPeer("net2", "shared")
	if err != nil {
		t.Fatalf("get peer in net2: %v", err)
	}
	if got.PublicKey != "key-b" {
		t.Errorf("public_key = %q, want key-b", got.PublicKey)
	}
}

func TestDeletePeer_RemovesPeer(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	testutil.SeedPeerDB(t, db, "testnet", "to-delete", "10.0.5.50/32", "del-key", false, false, false)

	if err := db.DeletePeer("testnet", "to-delete"); err != nil {
		t.Fatalf("delete peer: %v", err)
	}

	_, err := db.GetPeer("testnet", "to-delete")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected error after delete: %v", err)
	}
}

func TestDeletePeer_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	err := db.DeletePeer("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected error for nonexistent peer: %v", err)
	}
}

func TestUpdatePeer_Rename(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	testutil.SeedPeerDB(t, db, "testnet", "old-name", "10.0.5.60/32", "ren-key", false, false, false)

	newName := "new-name"
	peer, err := db.UpdatePeer("testnet", "old-name", service.PeerDiff{Name: &newName})
	if err != nil {
		t.Fatalf("update peer: %v", err)
	}
	if peer.Name != "new-name" {
		t.Errorf("name = %q, want new-name", peer.Name)
	}

	renamed, err := db.GetPeer("testnet", "new-name")
	if err != nil {
		t.Fatalf("get renamed peer: %v", err)
	}
	if renamed.Name != "new-name" {
		t.Fatalf("renamed peer = %q, want new-name", renamed.Name)
	}
}

func TestUpdatePeer_ToggleAdmin(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	testutil.SeedPeerDB(t, db, "testnet", "toggle-peer", "10.0.5.70/32", "tog-key", false, false, false)

	adminTrue := true
	peer, err := db.UpdatePeer("testnet", "toggle-peer", service.PeerDiff{Admin: &adminTrue})
	if err != nil {
		t.Fatalf("update peer admin: %v", err)
	}
	if !peer.Admin {
		t.Error("peer should be admin")
	}
}

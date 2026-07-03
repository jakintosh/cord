package database_test

import (
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetwork(t *testing.T, db *database.DB) {
	t.Helper()

	name := "testnet"
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main:       service.PlaneConfig{Name: name, Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 80},
			Invite:     service.PlaneConfig{Name: name + "-i", Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 80},
			CreatedAt:  time.Now(),
		},
		&service.Cidr{
			Name:   "testnet",
			Cidr:   "10.0.0.0/16",
			Length: 16,
			Prefix: 32,
		},
		&service.Peer{
			Name:      "cord-server",
			Cidr:      "10.0.0.1/32",
			PublicKey: "pub-test",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

func TestInsertAndGetPeer(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	peer := &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.5.1/32",
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}

	if err := db.InsertPeer("testnet", peer); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	got, err := db.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}

	if got.Name != peer.Name {
		t.Errorf("name = %q, want %q", got.Name, peer.Name)
	}
	if got.PublicKey != peer.PublicKey {
		t.Errorf("public_key = %q, want %q", got.PublicKey, peer.PublicKey)
	}
	if got.Cidr != peer.Cidr {
		t.Errorf("cidr = %q, want %q", got.Cidr, peer.Cidr)
	}
	if got.Admin != peer.Admin {
		t.Errorf("admin = %v, want %v", got.Admin, peer.Admin)
	}
	if got.Enabled != peer.Enabled {
		t.Errorf("enabled = %v, want %v", got.Enabled, peer.Enabled)
	}
	if got.Confirmed != peer.Confirmed {
		t.Errorf("confirmed = %v, want %v", got.Confirmed, peer.Confirmed)
	}
}

func TestGetPeer_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	_, err := db.GetPeer("testnet", "nobody")
	if err == nil {
		t.Fatal("expected error for nonexistent peer")
	}
}

func TestGetPeer_WrongNetwork(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-key",
		Cidr:      "10.0.5.1/32",
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	_, err := db.GetPeer("othernet", "alice")
	if err == nil {
		t.Fatal("expected error for wrong network")
	}
}

func TestGetPeerByIP(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "bob",
		PublicKey: "bob-key",
		Cidr:      "10.0.5.2/32",
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	got, err := db.GetPeerByIP("testnet", net.IPv4(10, 0, 5, 2))
	if err != nil {
		t.Fatalf("get peer by IP: %v", err)
	}
	if got.Name != "bob" {
		t.Errorf("name = %q, want bob", got.Name)
	}
}

func TestGetPeerByIP_NotConfirmed(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "unconfirmed",
		PublicKey: "unc-key",
		Cidr:      "10.0.5.3/32",
		Enabled:   true,
		Confirmed: false,
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	_, err := db.GetPeerByIP("testnet", net.IPv4(10, 0, 5, 3))
	if err == nil {
		t.Fatal("expected error for unconfirmed peer")
	}
}

func TestGetPeerByKey(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "charlie",
		PublicKey: "charlie-key",
		Cidr:      "10.0.5.4/32",
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	got, err := db.GetPeerByKey("testnet", "charlie-key")
	if err != nil {
		t.Fatalf("get peer by key: %v", err)
	}
	if got.Name != "charlie" {
		t.Errorf("name = %q, want charlie", got.Name)
	}
}

func TestGetPeerByKey_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	_, err := db.GetPeerByKey("testnet", "unknown-key")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestListPeers(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "ddd",
		PublicKey: "ddd-key",
		Cidr:      "10.0.5.10/32",
	}); err != nil {
		t.Fatalf("insert ddd: %v", err)
	}
	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "aaa",
		PublicKey: "aaa-key",
		Cidr:      "10.0.5.11/32",
	}); err != nil {
		t.Fatalf("insert aaa: %v", err)
	}

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

func TestListPeers_EmptyNetwork(t *testing.T) {
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

func TestInsertPeer_DuplicateName(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	peer := &service.Peer{
		Name:      "dup",
		PublicKey: "key-a",
		Cidr:      "10.0.5.20/32",
	}
	if err := db.InsertPeer("testnet", peer); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	peer.PublicKey = "key-b"
	peer.Cidr = "10.0.5.21/32"
	err := db.InsertPeer("testnet", peer)
	if err == nil {
		t.Fatal("expected error for duplicate peer name")
	}
}

func TestInsertPeer_DuplicateIP(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "peer1",
		PublicKey: "key1",
		Cidr:      "10.0.5.30/32",
	}); err != nil {
		t.Fatalf("insert peer1: %v", err)
	}

	err := db.InsertPeer("testnet", &service.Peer{
		Name:      "peer2",
		PublicKey: "key2",
		Cidr:      "10.0.5.30/32",
	})
	if err == nil {
		t.Fatal("expected error for duplicate IP")
	}
}

func TestInsertPeer_SameNameDifferentNetwork(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	now := time.Now()
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       "net2",
			PrivateKey: "priv2",
			PublicKey:  "pub2",
			ExternalIP: "1.1.1.2",
			Main: service.PlaneConfig{
				Name:          "net2",
				Cidr:          "172.16.0.0/16",
				WireguardPort: 51822,
				ApiPort:       8081,
			},
			Invite: service.PlaneConfig{
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
			Length: 16,
			Prefix: 32,
		},
		&service.Peer{
			Name:      "cord-server",
			Cidr:      "172.16.0.1/32",
			PublicKey: "pub2",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("insert net2: %v", err)
	}

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "shared",
		PublicKey: "key-a",
		Cidr:      "10.0.5.40/32",
	}); err != nil {
		t.Fatalf("insert in testnet: %v", err)
	}

	if err := db.InsertPeer("net2", &service.Peer{
		Name:      "shared",
		PublicKey: "key-b",
		Cidr:      "172.16.5.40/32",
	}); err != nil {
		t.Fatalf("insert in net2: %v", err)
	}

	got, err := db.GetPeer("net2", "shared")
	if err != nil {
		t.Fatalf("get peer in net2: %v", err)
	}
	if got.PublicKey != "key-b" {
		t.Errorf("public_key = %q, want key-b", got.PublicKey)
	}
}

func TestDeletePeer(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "to-delete",
		PublicKey: "del-key",
		Cidr:      "10.0.5.50/32",
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	if err := db.DeletePeer("testnet", "to-delete"); err != nil {
		t.Fatalf("delete peer: %v", err)
	}

	_, err := db.GetPeer("testnet", "to-delete")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeletePeer_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	err := db.DeletePeer("testnet", "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent peer")
	}
}

func TestUpdatePeer_Rename(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "old-name",
		PublicKey: "ren-key",
		Cidr:      "10.0.5.60/32",
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	newName := "new-name"
	peer, err := db.UpdatePeer("testnet", "old-name", &newName, nil, nil, nil)
	if err != nil {
		t.Fatalf("update peer: %v", err)
	}
	if peer.Name != "new-name" {
		t.Errorf("name = %q, want new-name", peer.Name)
	}

	exists, err := db.PeerExists("testnet", "new-name")
	if err != nil {
		t.Fatalf("peer exists: %v", err)
	}
	if !exists {
		t.Fatal("peer should exist under new name")
	}
}

func TestUpdatePeer_ToggleAdmin(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "toggle-peer",
		PublicKey: "tog-key",
		Cidr:      "10.0.5.70/32",
		Admin:     false,
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	adminTrue := true
	peer, err := db.UpdatePeer("testnet", "toggle-peer", nil, &adminTrue, nil, nil)
	if err != nil {
		t.Fatalf("update peer admin: %v", err)
	}
	if !peer.Admin {
		t.Error("peer should be admin")
	}
}

func TestPeerExists(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	if err := db.InsertPeer("testnet", &service.Peer{
		Name:      "exists",
		PublicKey: "exists-key",
		Cidr:      "10.0.5.80/32",
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	exists, err := db.PeerExists("testnet", "exists")
	if err != nil {
		t.Fatalf("peer exists: %v", err)
	}
	if !exists {
		t.Fatal("peer should exist")
	}

	exists, err = db.PeerExists("testnet", "nope")
	if err != nil {
		t.Fatalf("peer exists: %v", err)
	}
	if exists {
		t.Fatal("peer should not exist")
	}
}

func TestIPv6Peer(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetwork(t, db)

	peer := &service.Peer{
		Name:      "ipv6-peer",
		PublicKey: "v6-key",
		Cidr:      "fd00::1/128",
		Enabled:   true,
		Confirmed: true,
	}
	if err := db.InsertPeer("testnet", peer); err != nil {
		t.Fatalf("insert ipv6 peer: %v", err)
	}

	got, err := db.GetPeer("testnet", "ipv6-peer")
	if err != nil {
		t.Fatalf("get ipv6 peer: %v", err)
	}
	if got.Cidr != "fd00::1/128" {
		t.Errorf("cidr = %q, want fd00::1/128", got.Cidr)
	}

	gotByIP, err := db.GetPeerByIP("testnet", net.ParseIP("fd00::1"))
	if err != nil {
		t.Fatalf("get ipv6 peer by IP: %v", err)
	}
	if gotByIP.Name != "ipv6-peer" {
		t.Errorf("name = %q, want ipv6-peer", gotByIP.Name)
	}
}

package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestGetNetwork_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.GetNetwork("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent network")
	}
}

func TestInsertAndGetNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	createdAt := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	net := &service.Network{
		Name:             "homenet",
		PrivateKey:       "priv-key-123",
		PublicKey:        "pub-key-123",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "192.168.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        createdAt,
	}

	if err := db.BootstrapNetwork(net, &service.Cidr{Name: "homenet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub-key-123", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("insert network: %v", err)
	}

	got, err := db.GetNetwork("homenet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}

	if got.Name != net.Name {
		t.Errorf("name = %q, want %q", got.Name, net.Name)
	}
	if got.PrivateKey != net.PrivateKey {
		t.Errorf("private_key = %q, want %q", got.PrivateKey, net.PrivateKey)
	}
	if got.PublicKey != net.PublicKey {
		t.Errorf("public_key = %q, want %q", got.PublicKey, net.PublicKey)
	}
	if got.MainCidr != net.MainCidr {
		t.Errorf("main_cidr = %q, want %q", got.MainCidr, net.MainCidr)
	}
	if got.InviteCidr != net.InviteCidr {
		t.Errorf("invite_cidr = %q, want %q", got.InviteCidr, net.InviteCidr)
	}
	if got.ExternalIP != net.ExternalIP {
		t.Errorf("external_ip = %q, want %q", got.ExternalIP, net.ExternalIP)
	}
	if got.ListenPort != net.ListenPort {
		t.Errorf("listen_port = %d, want %d", got.ListenPort, net.ListenPort)
	}
	if got.InviteListenPort != net.InviteListenPort {
		t.Errorf("invite_listen_port = %d, want %d", got.InviteListenPort, net.InviteListenPort)
	}
	if got.ApiPort != net.ApiPort {
		t.Errorf("api_port = %d, want %d", got.ApiPort, net.ApiPort)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, createdAt)
	}
}

func TestInsertNetwork_Duplicate(t *testing.T) {
	db := testutil.SetupDB(t)

	net := &service.Network{
		Name:             "homenet",
		PrivateKey:       "priv-a",
		PublicKey:        "pub-a",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.1.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        time.Now(),
	}

	if err := db.BootstrapNetwork(net, &service.Cidr{Name: "homenet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub-a", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := db.BootstrapNetwork(net, &service.Cidr{Name: "homenet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub-a", Admin: true, Enabled: true, Confirmed: true})
	if err == nil {
		t.Fatal("expected error for duplicate network")
	}
}

func TestListNetworkNames(t *testing.T) {
	db := testutil.SetupDB(t)

	names, err := db.ListNetworkNames()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names, got %d", len(names))
	}

	now := time.Now()
	mustInsert := func(name string) {
		t.Helper()
		err := db.BootstrapNetwork(&service.Network{
			Name:             name,
			PrivateKey:       "priv-" + name,
			PublicKey:        "pub-" + name,
			MainCidr:         "10.0.0.0/16",
			InviteCidr:       "10.1.0.0/24",
			ExternalIP:       "1.1.1.1",
			ListenPort:       51820,
			InviteListenPort: 51821,
			ApiPort:          8080,
			CreatedAt:        now,
		}, &service.Cidr{Name: name, Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub-" + name, Admin: true, Enabled: true, Confirmed: true})
		if err != nil {
			t.Fatalf("insert %s: %v", name, err)
		}
	}

	mustInsert("alpha")
	mustInsert("beta")
	mustInsert("gamma")

	names, err = db.ListNetworkNames()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "gamma" {
		t.Fatalf("unexpected order: %v", names)
	}
}

func TestDeleteNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	now := time.Now()
	if err := db.BootstrapNetwork(&service.Network{
		Name:             "deleteme",
		PrivateKey:       "priv",
		PublicKey:        "pub",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.1.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        now,
	}, &service.Cidr{Name: "deleteme", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.DeleteNetwork("deleteme"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := db.GetNetwork("deleteme")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteNetwork_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.DeleteNetwork("ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent network")
	}
}

func TestDeleteNetwork_Cascade(t *testing.T) {
	db := testutil.SetupDB(t)

	now := time.Now()
	if err := db.BootstrapNetwork(&service.Network{
		Name:             "cascadenet",
		PrivateKey:       "priv",
		PublicKey:        "pub",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.1.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        now,
	}, &service.Cidr{Name: "cascadenet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("insert network: %v", err)
	}

	if err := db.InsertCidr("cascadenet", &service.Cidr{
		Name:   "subnet-1",
		Cidr:   "10.0.1.0/24",
		Length: 32,
		Prefix: 24,
	}); err != nil {
		t.Fatalf("insert cidr: %v", err)
	}

	if err := db.InsertPeer("cascadenet", &service.Peer{
		Name:      "peer-1",
		PublicKey: "peer-key-1",
		Cidr:      "10.0.1.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("insert peer: %v", err)
	}

	if err := db.DeleteNetwork("cascadenet"); err != nil {
		t.Fatalf("delete network: %v", err)
	}

	cidrs, err := db.ListCidrs("cascadenet")
	if err != nil {
		t.Fatalf("list cidrs after cascade: %v", err)
	}
	if len(cidrs) != 0 {
		t.Errorf("expected 0 cidrs after cascade delete, got %d", len(cidrs))
	}

	peers, err := db.ListPeers("cascadenet")
	if err != nil {
		t.Fatalf("list peers after cascade: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after cascade delete, got %d", len(peers))
	}
}

func TestOpenDatabase(t *testing.T) {
	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestOpenDatabase_UserVersionSet(t *testing.T) {
	db := testutil.SetupDB(t)

	var version int
	if err := db.Conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
	}
}

func TestOpenDatabase_TablesExist(t *testing.T) {
	db := testutil.SetupDB(t)

	for _, table := range []string{
		"network",
		"cidr",
		"peer",
		"association",
		"invite",
		"endpoint",
	} {
		var name string
		err := db.Conn.QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?1`,
			table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q should exist: %v", table, err)
		}
	}
}

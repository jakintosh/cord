package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func serverCidr() *service.Cidr {
	return &service.Cidr{
		Name:     "cord-server-cidr",
		Cidr:     "10.0.0.1/32",
		Prefix:   32,
		Bits:     32,
		Terminal: true,
	}
}

func serverPeer(pubKey string) *service.Peer {
	return &service.Peer{
		Name:      "cord-server",
		CidrName:  "cord-server-cidr",
		PublicKey: pubKey,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}
}

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
	net := &service.NetworkConfig{
		Name:       "homenet",
		PrivateKey: "priv-key-123",
		PublicKey:  "pub-key-123",
		ExternalIP: "192.168.1.1",
		Main: service.PlaneConfig{
			Name:          "homenet",
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		Invite: service.PlaneConfig{
			Name:          "homenet-i",
			Cidr:          "10.1.0.0/24",
			WireguardPort: 51821,
			ApiPort:       8080,
		},
		CreatedAt: createdAt,
	}

	if err := db.BootstrapNetwork(
		net,
		&service.Cidr{
			Name:   "homenet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		serverCidr(),
		serverPeer("pub-key-123"),
	); err != nil {
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
	if got.Main.Cidr != net.Main.Cidr {
		t.Errorf("main_cidr = %q, want %q", got.Main.Cidr, net.Main.Cidr)
	}
	if got.Invite.Cidr != net.Invite.Cidr {
		t.Errorf("invite_cidr = %q, want %q", got.Invite.Cidr, net.Invite.Cidr)
	}
	if got.ExternalIP != net.ExternalIP {
		t.Errorf("external_ip = %q, want %q", got.ExternalIP, net.ExternalIP)
	}
	if got.Main.WireguardPort != net.Main.WireguardPort {
		t.Errorf("main_wg_port = %d, want %d", got.Main.WireguardPort, net.Main.WireguardPort)
	}
	if got.Invite.WireguardPort != net.Invite.WireguardPort {
		t.Errorf("invite_wg_port = %d, want %d", got.Invite.WireguardPort, net.Invite.WireguardPort)
	}
	if got.Main.ApiPort != net.Main.ApiPort {
		t.Errorf("main_api_port = %d, want %d", got.Main.ApiPort, net.Main.ApiPort)
	}
	if got.Invite.ApiPort != net.Invite.ApiPort {
		t.Errorf("invite_api_port = %d, want %d", got.Invite.ApiPort, net.Invite.ApiPort)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, createdAt)
	}
}

func TestInsertNetwork_Duplicate(t *testing.T) {
	db := testutil.SetupDB(t)

	net := &service.NetworkConfig{
		Name:       "homenet",
		PrivateKey: "priv-a",
		PublicKey:  "pub-a",
		ExternalIP: "1.1.1.1",
		Main: service.PlaneConfig{
			Name:          "homenet",
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		Invite: service.PlaneConfig{
			Name:          "homenet-i",
			Cidr:          "10.1.0.0/24",
			WireguardPort: 51821,
			ApiPort:       8080,
		},
		CreatedAt: time.Now(),
	}

	if err := db.BootstrapNetwork(
		net,
		&service.Cidr{
			Name:   "homenet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		serverCidr(),
		serverPeer("pub-a"),
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := db.BootstrapNetwork(
		net,
		&service.Cidr{
			Name:   "homenet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		serverCidr(),
		serverPeer("pub-a"),
	)
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
		err := db.BootstrapNetwork(
			&service.NetworkConfig{
				Name:       name,
				PrivateKey: "priv-" + name,
				PublicKey:  "pub-" + name,
				ExternalIP: "1.1.1.1",
				Main: service.PlaneConfig{
					Name:          name,
					Cidr:          "10.0.0.0/16",
					WireguardPort: 51820,
					ApiPort:       8080,
				},
				Invite: service.PlaneConfig{
					Name:          "homenet-i",
					Cidr:          "10.1.0.0/24",
					WireguardPort: 51821,
					ApiPort:       8080,
				},
				CreatedAt: now,
			},
			&service.Cidr{
				Name:   name,
				Cidr:   "10.0.0.0/16",
				Prefix: 16,
				Bits:   32,
			},
			serverCidr(),
			serverPeer("pub-"+name),
		)
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

	networks, err := db.ListNetworks()
	if err != nil {
		t.Fatalf("list networks: %v", err)
	}
	if len(networks) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(networks))
	}
	if networks[0].Name != "alpha" || networks[1].Name != "beta" || networks[2].Name != "gamma" {
		t.Fatalf("unexpected network order: %v, %v, %v", networks[0].Name, networks[1].Name, networks[2].Name)
	}
}

func TestDeleteNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	now := time.Now()
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       "deleteme",
			PrivateKey: "priv",
			PublicKey:  "pub",
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          "deleteme",
				Cidr:          "10.0.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          "deleteme-i",
				Cidr:          "10.1.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: now,
		},
		&service.Cidr{
			Name:   "deleteme",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		serverCidr(),
		serverPeer("pub"),
	); err != nil {
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
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       "cascadenet",
			PrivateKey: "priv",
			PublicKey:  "pub",
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          "cascadenet",
				Cidr:          "10.0.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          "cascadenet-i",
				Cidr:          "10.1.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: now,
		},
		&service.Cidr{
			Name:   "cascadenet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		serverCidr(),
		serverPeer("pub"),
	); err != nil {
		t.Fatalf("insert network: %v", err)
	}

	testutil.SeedPeerDB(t, db, "cascadenet", "peer-1", "10.0.1.5/32", "peer-key-1", false, true, true)

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
		"group",
		"association",
		"peer",
		"cidr",
		"cidr_assignment",
		"registration",
		"registration_assignment",
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

package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func serverCidr() *service.Cidr {
	return &service.Cidr{
		Name:     "cord-server",
		Cidr:     "10.0.0.1/32",
		Prefix:   32,
		Bits:     32,
		Terminal: true,
	}
}

func seedDatabaseNetwork(
	t *testing.T,
	db *database.DB,
	name string,
) {
	t.Helper()
	network := &service.Network{
		Name:       name,
		PrivateKey: "priv-" + name,
		PublicKey:  "pub-" + name,
		ExternalIP: "1.1.1.1",
		Main: service.Plane{
			Name:          name,
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		Invite: service.Plane{
			Name:          name + "-i",
			Cidr:          "10.1.0.0/24",
			WireguardPort: 51821,
			ApiPort:       8080,
		},
		CreatedAt: time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	}
	if err := db.BootstrapNetwork(network,
		&service.Cidr{Name: name, Cidr: network.Main.Cidr, Prefix: 16, Bits: 32},
		serverCidr(), serverPeer(network.PublicKey),
	); err != nil {
		t.Fatalf("bootstrap %s: %v", name, err)
	}
}

func serverPeer(
	pubKey string,
) *service.Peer {
	return &service.Peer{
		Name:      "cord-server",
		CidrName:  "cord-server",
		PublicKey: pubKey,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}
}

func TestGetNetwork_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	_, err := db.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("get missing network = %v, want ErrNotFound", err)
	}
}

func TestBootstrapNetwork_RoundTrip(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	createdAt := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	net := &service.Network{
		Name:       "homenet",
		PrivateKey: "priv-key-123",
		PublicKey:  "pub-key-123",
		ExternalIP: "192.168.1.1",
		Main: service.Plane{
			Name:          "homenet",
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		Invite: service.Plane{
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

func TestBootstrapNetwork_Duplicate(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	net := &service.Network{
		Name:       "homenet",
		PrivateKey: "priv-a",
		PublicKey:  "pub-a",
		ExternalIP: "1.1.1.1",
		Main: service.Plane{
			Name:          "homenet",
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		Invite: service.Plane{
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
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("duplicate network = %v, want ErrConflict", err)
	}
	got, err := db.GetNetwork("homenet")
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicKey != net.PublicKey || got.PrivateKey != net.PrivateKey || !got.CreatedAt.Equal(net.CreatedAt.Truncate(time.Second)) {
		t.Fatalf("duplicate bootstrap changed network: %+v", got)
	}
}

func TestListNetworks_SortedByName(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	networks, err := db.ListNetworks()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(networks))
	}

	for _, name := range []string{"gamma", "alpha", "beta"} {
		seedDatabaseNetwork(t, db, name)
	}

	networks, err = db.ListNetworks()
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

func TestDeleteNetwork_RemovesNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	seedDatabaseNetwork(t, db, "deleteme")

	if err := db.DeleteNetwork("deleteme"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := db.GetNetwork("deleteme")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected error after delete: %v", err)
	}
}

func TestDeleteNetwork_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	err := db.DeleteNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected error for nonexistent network: %v", err)
	}
}

func TestDeleteNetwork_RefusesEnabled(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	seedDatabaseNetwork(t, db, "enablednet")

	if err := db.SetNetworkEnabled("enablednet", true); err != nil {
		t.Fatalf("enable: %v", err)
	}

	err := db.DeleteNetwork("enablednet")
	if !errors.Is(err, service.ErrNetworkEnabled) {
		t.Fatalf("err = %v, want ErrNetworkEnabled", err)
	}

	if _, err := db.GetNetwork("enablednet"); err != nil {
		t.Fatalf("network should survive a refused delete: %v", err)
	}
}

func TestDeleteNetwork_Cascade(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	seedDatabaseNetwork(t, db, "cascadenet")

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

func TestBootstrapNetwork_CidrConflictRollsBackAndCanRetry(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	network := &service.Network{
		Name:      "rollback",
		Main:      service.Plane{Name: "rollback", Cidr: "10.0.0.0/16"},
		Invite:    service.Plane{Name: "rollback-i", Cidr: "10.1.0.0/24"},
		CreatedAt: time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	}
	root := &service.Cidr{Name: "rollback", Cidr: "10.0.0.0/16"}
	server := serverCidr()
	server.Name = root.Name
	if err := db.BootstrapNetwork(network, root, server, serverPeer("key")); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("conflicting CIDR = %v, want ErrConflict", err)
	}
	if _, err := db.GetNetwork("rollback"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("network after rollback = %v", err)
	}
	cidrs, err := db.ListCidrs("rollback")
	if err != nil {
		t.Fatal(err)
	}
	if len(cidrs) != 0 {
		t.Fatalf("CIDRs after rollback = %+v", cidrs)
	}
	if err := db.BootstrapNetwork(network, root, serverCidr(), serverPeer("key")); err != nil {
		t.Fatalf("retry: %v", err)
	}
	for _, enabled := range []bool{true, false} {
		if err := db.SetNetworkEnabled("rollback", enabled); err != nil {
			t.Fatal(err)
		}
		got, err := db.GetNetwork("rollback")
		if err != nil {
			t.Fatal(err)
		}
		if got.Enabled != enabled {
			t.Fatalf("enabled = %v, want %v", got.Enabled, enabled)
		}
	}
}

func seedNetwork(
	t *testing.T,
	db *database.DB,
) {
	t.Helper()
	seedDatabaseNetwork(t, db, "testnet")
}

func seedNetworkForCidr(
	t *testing.T,
	db *database.DB,
) {
	t.Helper()
	seedDatabaseNetwork(t, db, "cidrnet")
}

func seedNetworkForRegistration(
	t *testing.T,
	db *database.DB,
) {
	t.Helper()
	seedNetworkForRegistrationWithInviteCIDR(t, db, "10.1.0.0/24")
}

func seedNetworkForRegistrationWithInviteCIDR(
	t *testing.T,
	db *database.DB,
	inviteCIDR string,
) {
	t.Helper()

	name := "regnet"
	if err := db.BootstrapNetwork(
		&service.Network{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main: service.Plane{
				Name:          name,
				Cidr:          "10.0.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.Plane{
				Name:          name + "-i",
				Cidr:          inviteCIDR,
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: time.Now(),
		},
		&service.Cidr{
			Name:   "regnet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server",
			Cidr:     "10.0.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server",
			PublicKey: "pub",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

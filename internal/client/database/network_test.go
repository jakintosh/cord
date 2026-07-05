package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
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
	net := &service.NetworkConfig{
		Name:          "homenet",
		PrivateKey:    "priv-key-123",
		InterfaceName: "wg-homenet",
		AssignedRoute: "10.42.0.5/32",
		Server: service.ServerInfo{
			PublicKey: "server-pub-key",
			Endpoint:  "1.2.3.4:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		Enabled:   true,
		CreatedAt: createdAt,
	}

	if err := db.InsertNetwork(net); err != nil {
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
	if got.AssignedRoute != net.AssignedRoute {
		t.Errorf("assigned_cidr = %q, want %q", got.AssignedRoute, net.AssignedRoute)
	}
	if got.Server.PublicKey != net.Server.PublicKey {
		t.Errorf("server_pubkey = %q, want %q", got.Server.PublicKey, net.Server.PublicKey)
	}
	if got.Server.Endpoint != net.Server.Endpoint {
		t.Errorf("server_endpoint = %q, want %q", got.Server.Endpoint, net.Server.Endpoint)
	}
	if got.Server.Route != net.Server.Route {
		t.Errorf("server_route = %q, want %q", got.Server.Route, net.Server.Route)
	}
	if got.Server.APIPort != net.Server.APIPort {
		t.Errorf("server_api_port = %d, want %d", got.Server.APIPort, net.Server.APIPort)
	}
	if got.Enabled != net.Enabled {
		t.Errorf("enabled = %v, want %v", got.Enabled, net.Enabled)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, createdAt)
	}
}

func TestInsertAndGetNetwork_Disabled(t *testing.T) {
	db := testutil.SetupDB(t)

	net := &service.NetworkConfig{
		Name:          "offnet",
		PrivateKey:    "priv-off",
		InterfaceName: "wg-offnet",
		AssignedRoute: "10.42.0.6/16",
		Server: service.ServerInfo{
			PublicKey: "server-pub-key",
			Endpoint:  "1.2.3.4:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		Enabled:   false,
		CreatedAt: time.Now(),
	}

	if err := db.InsertNetwork(net); err != nil {
		t.Fatalf("insert network: %v", err)
	}

	got, err := db.GetNetwork("offnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if got.Enabled {
		t.Error("expected enabled=false after insert with Enabled=false")
	}
}

func TestInsertNetwork_Duplicate(t *testing.T) {
	db := testutil.SetupDB(t)

	net := &service.NetworkConfig{
		Name:          "homenet",
		PrivateKey:    "priv-a",
		InterfaceName: "wg-homenet",
		AssignedRoute: "10.0.0.5/16",
		Server: service.ServerInfo{
			PublicKey: "srv-key",
			Endpoint:  "1.1.1.1:51820",
			Route:     "10.0.0.1/32",
			APIPort:   8443,
		},
		CreatedAt: time.Now(),
	}

	if err := db.InsertNetwork(net); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := db.InsertNetwork(net)
	if err == nil {
		t.Fatal("expected error for duplicate network")
	}
}

func TestListNetworkNames_Empty(t *testing.T) {
	db := testutil.SetupDB(t)

	names, err := db.ListNetworkNames()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names, got %d", len(names))
	}
}

func TestListNetworkNames_Ordered(t *testing.T) {
	db := testutil.SetupDB(t)

	now := time.Now()
	mustInsert := func(name string) {
		t.Helper()
		err := db.InsertNetwork(&service.NetworkConfig{
			Name:          name,
			PrivateKey:    "priv-" + name,
			InterfaceName: "wg-" + name,
			AssignedRoute: "10.42.0.0/16",
			Server: service.ServerInfo{
				PublicKey: "srv-key",
				Endpoint:  "1.1.1.1:51820",
				Route:     "10.42.0.1/32",
				APIPort:   8443,
			},
			CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("insert %s: %v", name, err)
		}
	}

	mustInsert("beta")
	mustInsert("alpha")
	mustInsert("gamma")

	names, err := db.ListNetworkNames()
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
	if err := db.InsertNetwork(&service.NetworkConfig{
		Name:          "deleteme",
		PrivateKey:    "priv",
		InterfaceName: "wg-deleteme",
		AssignedRoute: "10.42.0.5/16",
		Server: service.ServerInfo{
			PublicKey: "srv-key",
			Endpoint:  "1.1.1.1:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		CreatedAt: now,
	}); err != nil {
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
	if err := db.InsertNetwork(&service.NetworkConfig{
		Name:          "cascadenet",
		PrivateKey:    "priv",
		InterfaceName: "wg-cascadenet",
		AssignedRoute: "10.42.0.5/16",
		Server: service.ServerInfo{
			PublicKey: "srv-key",
			Endpoint:  "1.1.1.1:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert network: %v", err)
	}

	if err := db.SetPeers("cascadenet", []service.Peer{
		{Name: "peer-1", PublicKey: "peer-key-1", Route: "10.42.1.5/32"},
		{Name: "peer-2", PublicKey: "peer-key-2", Route: "10.42.1.6/32"},
	}); err != nil {
		t.Fatalf("reconcile peers: %v", err)
	}

	if err := db.DeleteNetwork("cascadenet"); err != nil {
		t.Fatalf("delete network: %v", err)
	}

	peers, err := db.ListPeers("cascadenet")
	if err != nil {
		t.Fatalf("list peers after cascade: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after cascade delete, got %d", len(peers))
	}
}

func TestSetNetworkEnabled(t *testing.T) {
	db := testutil.SetupDB(t)

	net := &service.NetworkConfig{
		Name:          "toggle",
		PrivateKey:    "priv",
		InterfaceName: "wg-toggle",
		AssignedRoute: "10.42.0.5/16",
		Server: service.ServerInfo{
			PublicKey: "srv-key",
			Endpoint:  "1.1.1.1:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		Enabled:   false,
		CreatedAt: time.Now(),
	}
	if err := db.InsertNetwork(net); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.SetNetworkEnabled("toggle", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	got, err := db.GetNetwork("toggle")
	if err != nil {
		t.Fatalf("get after enable: %v", err)
	}
	if !got.Enabled {
		t.Error("expected enabled=true after SetNetworkEnabled(true)")
	}

	if err := db.SetNetworkEnabled("toggle", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got, err = db.GetNetwork("toggle")
	if err != nil {
		t.Fatalf("get after disable: %v", err)
	}
	if got.Enabled {
		t.Error("expected enabled=false after SetNetworkEnabled(false)")
	}
}

func TestSetNetworkEnabled_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.SetNetworkEnabled("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent network")
	}
}

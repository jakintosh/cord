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
	net := &service.Network{
		Name:           "homenet",
		PrivateKey:     "priv-key-123",
		PublicKey:      "pub-key-123",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "server-pub-key",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		Enabled:        true,
		CreatedAt:      createdAt,
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
	if got.PublicKey != net.PublicKey {
		t.Errorf("public_key = %q, want %q", got.PublicKey, net.PublicKey)
	}
	if got.AssignedCidr != net.AssignedCidr {
		t.Errorf("assigned_cidr = %q, want %q", got.AssignedCidr, net.AssignedCidr)
	}
	if got.ServerPubkey != net.ServerPubkey {
		t.Errorf("server_pubkey = %q, want %q", got.ServerPubkey, net.ServerPubkey)
	}
	if got.ServerEndpoint != net.ServerEndpoint {
		t.Errorf("server_endpoint = %q, want %q", got.ServerEndpoint, net.ServerEndpoint)
	}
	if got.ServerApiAddr != net.ServerApiAddr {
		t.Errorf("server_api_addr = %q, want %q", got.ServerApiAddr, net.ServerApiAddr)
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

	net := &service.Network{
		Name:           "offnet",
		PrivateKey:     "priv-off",
		PublicKey:      "pub-off",
		AssignedCidr:   "10.42.0.6/16",
		ServerPubkey:   "server-pub-key",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		Enabled:        false,
		CreatedAt:      time.Now(),
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

	net := &service.Network{
		Name:           "homenet",
		PrivateKey:     "priv-a",
		PublicKey:      "pub-a",
		AssignedCidr:   "10.0.0.5/16",
		ServerPubkey:   "srv-key",
		ServerEndpoint: "1.1.1.1:51820",
		ServerApiAddr:  "10.0.0.1:8443",
		CreatedAt:      time.Now(),
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
		err := db.InsertNetwork(&service.Network{
			Name:           name,
			PrivateKey:     "priv-" + name,
			PublicKey:      "pub-" + name,
			AssignedCidr:   "10.42.0.0/16",
			ServerPubkey:   "srv-key",
			ServerEndpoint: "1.1.1.1:51820",
			ServerApiAddr:  "10.42.0.1:8443",
			CreatedAt:      now,
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
	if err := db.InsertNetwork(&service.Network{
		Name:           "deleteme",
		PrivateKey:     "priv",
		PublicKey:      "pub",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv-key",
		ServerEndpoint: "1.1.1.1:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		CreatedAt:      now,
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
	if err := db.InsertNetwork(&service.Network{
		Name:           "cascadenet",
		PrivateKey:     "priv",
		PublicKey:      "pub",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv-key",
		ServerEndpoint: "1.1.1.1:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("insert network: %v", err)
	}

	if err := db.SetPeers("cascadenet", []service.Peer{
		{Name: "peer-1", PublicKey: "peer-key-1", Cidr: "10.42.1.5/32"},
		{Name: "peer-2", PublicKey: "peer-key-2", Cidr: "10.42.1.6/32"},
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

func TestUpdateNetwork_EnableDisable(t *testing.T) {
	db := testutil.SetupDB(t)

	net := &service.Network{
		Name:           "toggle",
		PrivateKey:     "priv",
		PublicKey:      "pub",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv-key",
		ServerEndpoint: "1.1.1.1:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		Enabled:        false,
		CreatedAt:      time.Now(),
	}
	if err := db.InsertNetwork(net); err != nil {
		t.Fatalf("insert: %v", err)
	}

	enable := true
	got, err := db.UpdateNetwork("toggle", service.UpdateNetworkRequest{Enabled: &enable})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !got.Enabled {
		t.Error("expected enabled=true after update")
	}

	disable := false
	got, err = db.UpdateNetwork("toggle", service.UpdateNetworkRequest{Enabled: &disable})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if got.Enabled {
		t.Error("expected enabled=false after update")
	}
}

func TestUpdateNetwork_NilRequest(t *testing.T) {
	db := testutil.SetupDB(t)

	createdAt := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	net := &service.Network{
		Name:           "nochange",
		PrivateKey:     "priv",
		PublicKey:      "pub",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv-key",
		ServerEndpoint: "1.1.1.1:51820",
		ServerApiAddr:  "10.42.0.1:8443",
		Enabled:        false,
		CreatedAt:      createdAt,
	}
	if err := db.InsertNetwork(net); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := db.UpdateNetwork("nochange", service.UpdateNetworkRequest{})
	if err != nil {
		t.Fatalf("update with empty request: %v", err)
	}
	if got.Enabled {
		t.Error("enabled should remain false with nil request")
	}
}

package database_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestGetNetwork_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetNetwork_RoundTripsConfirmedNetwork(t *testing.T) {
	db := testutil.SetupDB(t)
	want := testutil.SeedNetworkDirect(t, db, "homenet")

	got, err := db.GetNetwork("homenet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}

	if got.Name != want.Name {
		t.Errorf("name = %q, want %q", got.Name, want.Name)
	}
	if got.PrivateKey != want.PrivateKey {
		t.Errorf("private key = %q, want %q", got.PrivateKey, want.PrivateKey)
	}
	if got.AssignedRoute != want.AssignedRoute {
		t.Errorf("assigned route = %q, want %q", got.AssignedRoute, want.AssignedRoute)
	}
	if got.Server != want.Server {
		t.Errorf("server = %#v, want %#v", got.Server, want.Server)
	}
	if got.ListenPort != want.ListenPort {
		t.Errorf("listen port = %d, want %d", got.ListenPort, want.ListenPort)
	}
	if got.Enabled {
		t.Error("seeded network should be disabled")
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("created at = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestListNetworks_Empty(t *testing.T) {
	db := testutil.SetupDB(t)

	networks, err := db.ListNetworks()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(networks) != 0 {
		t.Fatalf("networks = %d, want 0", len(networks))
	}
}

func TestListNetworks_OrderedWithFields(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "beta")
	testutil.SeedNetworkDirect(t, db, "alpha")
	if err := db.SetNetworkEnabled("alpha", true); err != nil {
		t.Fatalf("enable alpha: %v", err)
	}

	networks, err := db.ListNetworks()
	if err != nil {
		t.Fatalf("list networks: %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("networks = %d, want 2", len(networks))
	}
	if networks[0].Name != "alpha" || networks[1].Name != "beta" {
		t.Fatalf("unexpected order: %q, %q", networks[0].Name, networks[1].Name)
	}
	if !networks[0].Enabled {
		t.Error("alpha should be enabled")
	}
	if networks[1].Enabled {
		t.Error("beta should be disabled")
	}
}

func TestDeleteNetworkState_CascadesPeersAndEndpoints(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "cascadenet")
	reconciliation := testutil.NetworkReconciliation(service.PeerObservation{
		Peer: service.Peer{
			Name:      "peer-1",
			PublicKey: "peer-key-1",
			Route:     "10.42.1.5/32",
		},
		Endpoints: []service.PeerEndpoint{{
			Endpoint:         "1.2.3.4:51820",
			ServerObservedAt: testutil.FixedTime,
		}},
	})
	err := db.ApplyNetworkReconciliation("cascadenet", reconciliation)
	if err != nil {
		t.Fatalf("apply network reconciliation: %v", err)
	}

	if err := db.DeleteNetworkState("cascadenet"); err != nil {
		t.Fatalf("delete network state: %v", err)
	}

	var peers int
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM peer`).Scan(&peers); err != nil {
		t.Fatalf("count peers: %v", err)
	}
	if peers != 0 {
		t.Errorf("peers after delete = %d, want 0", peers)
	}
	var endpoints int
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM endpoint`).Scan(&endpoints); err != nil {
		t.Fatalf("count endpoints: %v", err)
	}
	if endpoints != 0 {
		t.Errorf("endpoints after delete = %d, want 0", endpoints)
	}
}

func TestSetNetworkEnabled(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "toggle")

	if err := db.SetNetworkEnabled("toggle", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	got, err := db.GetNetwork("toggle")
	if err != nil {
		t.Fatalf("get after enable: %v", err)
	}
	if !got.Enabled {
		t.Error("network should be enabled")
	}

	if err := db.SetNetworkEnabled("toggle", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got, err = db.GetNetwork("toggle")
	if err != nil {
		t.Fatalf("get after disable: %v", err)
	}
	if got.Enabled {
		t.Error("network should be disabled")
	}
}

func TestSetNetworkEnabled_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.SetNetworkEnabled("nonexistent", true)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateNetwork(t *testing.T) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "portnet")

	listenPort := uint16(51821)
	if err := db.UpdateNetwork(
		"portnet",
		service.NetworkOptions{ListenPort: &listenPort},
	); err != nil {
		t.Fatalf("update network: %v", err)
	}

	got, err := db.GetNetwork("portnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if got.ListenPort != listenPort {
		t.Errorf("listen port = %d, want %d", got.ListenPort, listenPort)
	}
}

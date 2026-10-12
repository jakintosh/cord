package database_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestGetNetwork_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	_, err := db.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetNetwork_RoundTripsConfirmedNetwork(
	t *testing.T,
) {
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

func TestListNetworks_Empty(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	networks, err := db.ListNetworks()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(networks) != 0 {
		t.Fatalf("networks = %d, want 0", len(networks))
	}
}

func TestListNetworks_OrderedWithFields(
	t *testing.T,
) {
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

func TestDeleteNetworkState_CascadesPeersAndEndpoints(
	t *testing.T,
) {
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

	// Endpoint reads join through peers, so public reads cannot reveal an
	// orphaned endpoint row. Inspect that storage invariant directly.
	var endpointsRemaining int
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM endpoint`).Scan(&endpointsRemaining); err != nil {
		t.Fatalf("count endpoint rows after delete: %v", err)
	}
	if endpointsRemaining != 0 {
		t.Fatalf("endpoint rows after delete = %d, want 0", endpointsRemaining)
	}

	testutil.SeedNetworkDirect(t, db, "cascadenet")
	peers, err := db.ListPeers("cascadenet")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Fatalf("peers after reinstall = %#v, want empty", peers)
	}
	endpoints, err := db.ListPeerEndpoints("cascadenet", "peer-key-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("endpoints after reinstall = %#v, want empty", endpoints)
	}
	if _, err := db.GetNetworkTopology("cascadenet"); !errors.Is(err, service.ErrTopologyUnavailable) {
		t.Fatalf("topology after reinstall = %v, want ErrTopologyUnavailable", err)
	}
}

func TestSetNetworkEnabled_PersistsIntent(
	t *testing.T,
) {
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

func TestSetNetworkEnabled_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	err := db.SetNetworkEnabled("nonexistent", true)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateNetwork_ChangesListenPort(
	t *testing.T,
) {
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

func TestUpdateNetwork_RejectsMissingPortWithoutChanges(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	want := testutil.SeedNetworkDirect(t, db, "testnet")
	err := db.UpdateNetwork("testnet", service.NetworkOptions{})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
	got, err := db.GetNetwork("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if got.ListenPort != want.ListenPort || got.PrivateKey != want.PrivateKey ||
		got.Enabled != want.Enabled || got.Server != want.Server {
		t.Fatalf("network changed after rejection: %#v", got)
	}
}

func TestUpdateNetwork_MissingNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	port := uint16(51820)
	err := db.UpdateNetwork("missing", service.NetworkOptions{ListenPort: &port})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
func TestDeleteNetworkState_Install(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")

	if err := db.DeleteNetworkState("testnet"); err != nil {
		t.Fatalf("delete install state: %v", err)
	}
	if _, err := db.GetInstall("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("install after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetworkState_Network(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	redeemInstall(t, db, "testnet")
	if _, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime); err != nil {
		t.Fatalf("confirm install: %v", err)
	}

	if err := db.DeleteNetworkState("testnet"); err != nil {
		t.Fatalf("delete network state: %v", err)
	}
	if _, err := db.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("network after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetworkState_NotFound(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	err := db.DeleteNetworkState("missing")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

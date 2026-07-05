package service_test

import (
	"errors"
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// TestEnableNetwork_AppliesCachedPeersSynchronously verifies that the
// cached peer set is applied to the device by the time EnableNetwork
// returns, without waiting for a sync tick.
func TestEnableNetwork_AppliesCachedPeersSynchronously(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "cached-peers")

	peerKey := mustGenKey(t)
	if err := env.Database.SetPeers("cached-peers", []service.Peer{{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	}}); err != nil {
		t.Fatalf("seed peers: %v", err)
	}

	if err := env.Service.EnableNetwork("cached-peers"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	dev := env.Backend.Device("cached-peers")
	if dev == nil {
		t.Fatal("expected device was created")
	}

	found := false
	for _, op := range dev.AppliedOps() {
		if op.Target.PublicKey.String() == peerKey {
			found = true
		}
	}
	if !found {
		t.Errorf("cached peer %q not applied to device after enable", peerKey)
	}
}

func TestBuildPeers_IncludesServer(t *testing.T) {
	env := testutil.SetupService(t)

	testutil.SeedNetworkDirect(t, env.Service, "peer-test")

	err := env.Service.EnableNetwork("peer-test")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	if env.Backend.Device("peer-test") == nil {
		t.Fatal("expected device was created")
	}

	ops := env.Backend.AppliedOpsFor("peer-test")
	if len(ops) != 1 {
		t.Fatalf("expected 1 peer op (server), got %d", len(ops))
	}
}

func TestBuildPeers_DoesNotIncludeSelf(t *testing.T) {
	env := testutil.SetupService(t)

	testutil.SeedNetworkDirect(t, env.Service, "self-test")

	err := env.Service.EnableNetwork("self-test")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	if env.Backend.Device("self-test") == nil {
		t.Fatal("expected device was created")
	}

	ops := env.Backend.AppliedOpsFor("self-test")
	if len(ops) != 1 {
		t.Fatalf("expected 1 peer op (server only), got %d", len(ops))
	}
}

func TestListPeers_Empty(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "empty-peers")

	peers, err := env.Service.ListPeers("empty-peers")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestListPeers_NetworkNotFound(t *testing.T) {
	env := testutil.SetupService(t)

	peers, err := env.Service.ListPeers("nonexistent")
	if err != nil {
		t.Fatalf("list peers for nonexistent network: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers for nonexistent network, got %d", len(peers))
	}
}

func TestListPeers_EmptyForNewNetwork(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "count-test")

	peers, err := env.Service.ListPeers("count-test")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("peer count = %d, want 0 before any fetch", len(peers))
	}
}

func TestListPeerStatus_NetworkNotFound(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.ListPeerStatus("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListPeerStatus_NotRunning_ReturnsCachedWithZeroRuntimeFields(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "not-running")

	peerKey := mustGenKey(t)
	if err := env.Database.SetPeers("not-running", []service.Peer{{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	}}); err != nil {
		t.Fatalf("seed peers: %v", err)
	}

	statuses, err := env.Service.ListPeerStatus("not-running")
	if err != nil {
		t.Fatalf("list peer status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(statuses))
	}
	got := statuses[0]
	if got.Name != "alice" {
		t.Errorf("name = %q, want alice", got.Name)
	}
	if got.Route != "10.42.0.9/32" {
		t.Errorf("route = %q, want 10.42.0.9/32", got.Route)
	}
	if got.Connected {
		t.Error("expected connected=false for a non-running network")
	}
	if got.Endpoint != "" {
		t.Errorf("endpoint = %q, want empty", got.Endpoint)
	}
	if !got.LastHandshake.IsZero() {
		t.Errorf("last_handshake = %v, want zero", got.LastHandshake)
	}
}

func TestListPeerStatus_Running_JoinsLiveDeviceState(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "running-net")

	peerKey := mustGenKey(t)
	if err := env.Database.SetPeers("running-net", []service.Peer{{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	}}); err != nil {
		t.Fatalf("seed peers: %v", err)
	}

	if err := env.Service.EnableNetwork("running-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	dev := env.Backend.Device("running-net")
	if dev == nil {
		t.Fatal("expected device was created")
	}

	parsedKey, err := wgtypes.ParseKey(peerKey)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	now := testutil.FixedTime
	dev.SetPeers(wireguard.PeerStatus{
		PublicKey:     parsedKey,
		Endpoint:      &net.UDPAddr{IP: net.ParseIP("5.6.7.8"), Port: 51820},
		LastHandshake: now,
	})

	statuses, err := env.Service.ListPeerStatus("running-net")
	if err != nil {
		t.Fatalf("list peer status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(statuses))
	}
	got := statuses[0]
	if got.Endpoint != "5.6.7.8:51820" {
		t.Errorf("endpoint = %q, want 5.6.7.8:51820", got.Endpoint)
	}
	if !got.LastHandshake.Equal(now) {
		t.Errorf("last_handshake = %v, want %v", got.LastHandshake, now)
	}
	if !got.Connected {
		t.Error("expected connected=true for a fresh handshake")
	}
}

func TestListPeerStatus_Running_StaleHandshakeNotConnected(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "stale-net")

	peerKey := mustGenKey(t)
	if err := env.Database.SetPeers("stale-net", []service.Peer{{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	}}); err != nil {
		t.Fatalf("seed peers: %v", err)
	}

	if err := env.Service.EnableNetwork("stale-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	dev := env.Backend.Device("stale-net")
	if dev == nil {
		t.Fatal("expected device was created")
	}

	parsedKey, err := wgtypes.ParseKey(peerKey)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	stale := testutil.FixedTime.Add(-service.StaleThreshold * 2)
	dev.SetPeers(wireguard.PeerStatus{
		PublicKey:     parsedKey,
		LastHandshake: stale,
	})

	statuses, err := env.Service.ListPeerStatus("stale-net")
	if err != nil {
		t.Fatalf("list peer status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(statuses))
	}
	if statuses[0].Connected {
		t.Error("expected connected=false for a stale handshake")
	}
}

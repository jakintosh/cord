package service_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
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

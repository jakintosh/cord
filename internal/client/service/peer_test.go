package service_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestBuildPeers_IncludesServer(t *testing.T) {
	env := testutil.SetupService(t)

	testutil.SeedNetworkDirect(t, env.Service, "peer-test")

	err := env.Service.EnableNetwork(t.Context(), "peer-test")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	if _, ok := env.Backend.UpConfigs["peer-test"]; !ok {
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

	err := env.Service.EnableNetwork(t.Context(), "self-test")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	if _, ok := env.Backend.UpConfigs["self-test"]; !ok {
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

func TestNetworkStatus_HasPeerCount(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "count-test")

	statuses, err := env.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, st := range statuses {
		if st.PeerCount != 0 {
			t.Errorf("peer_count = %d, want 0 before any fetch", st.PeerCount)
		}
	}
}

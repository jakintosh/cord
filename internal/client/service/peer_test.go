package service_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestBuildPeers_IncludesServer(t *testing.T) {
	env := testutil.SetupService(t)

	// buildPeers is unexported, but we can test it indirectly by
	// enabling a network and inspecting the device's applied peers.
	testutil.SeedNetworkDirect(t, env.Service, "peer-test")

	err := env.Service.EnableNetwork(t.Context(), "peer-test")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	d, ok := env.WireGuard.Devices["peer-test"]
	if !ok {
		t.Fatal("expected device was created")
	}

	peers := d.AppliedPeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer (server), got %d", len(peers))
	}

	serverPeer := peers[0]
	if serverPeer.PublicKey != "server-pub-key" {
		t.Errorf("public_key = %q, want server-pub-key", serverPeer.PublicKey)
	}
	if len(serverPeer.AllowedIPs) != 1 {
		t.Fatalf("expected 1 allowed ip, got %d", len(serverPeer.AllowedIPs))
	}
	if serverPeer.AllowedIPs[0] != "10.42.0.5/16" {
		t.Errorf("allowed_ips[0] = %q, want 10.42.0.5/16", serverPeer.AllowedIPs[0])
	}
	if serverPeer.Endpoint != "1.2.3.4:51820" {
		t.Errorf("endpoint = %q, want 1.2.3.4:51820", serverPeer.Endpoint)
	}
}

func TestBuildPeers_DoesNotIncludeSelf(t *testing.T) {
	env := testutil.SetupService(t)

	testutil.SeedNetworkDirect(t, env.Service, "self-test")

	err := env.Service.EnableNetwork(t.Context(), "self-test")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	d, ok := env.WireGuard.Devices["self-test"]
	if !ok {
		t.Fatal("expected device was created")
	}

	peers := d.AppliedPeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer (server only), got %d", len(peers))
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

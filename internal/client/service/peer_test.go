package service_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func mustGenKey(t *testing.T) string {
	t.Helper()
	k, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func TestListPeers_Empty(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "empty-peers")

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

	_, err := env.Service.ListPeers("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListPeers_CachedPeers(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "cached")

	peerKey := mustGenKey(t)
	testutil.SeedPeers(t, env.Database, "cached", service.Peer{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	})

	peers, err := env.Service.ListPeers("cached")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].Name != "alice" {
		t.Errorf("name = %q, want alice", peers[0].Name)
	}
	if peers[0].PublicKey != peerKey {
		t.Errorf("public key = %q, want %q", peers[0].PublicKey, peerKey)
	}
}

// TestRecordLocalEndpoint_FeedsTheCatalog verifies that an endpoint the
// runtime observed becomes a rotation candidate for that peer.
func TestRecordLocalEndpoint_FeedsTheCatalog(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "endpoints")

	peerKey := mustGenKey(t)
	testutil.SeedPeers(t, env.Database, "endpoints", service.Peer{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	})

	if err := env.Service.RecordLocalEndpoint(
		"endpoints",
		peerKey,
		"5.6.7.8:51820",
		testutil.FixedTime,
	); err != nil {
		t.Fatalf("record local endpoint: %v", err)
	}

	endpoints, err := env.Service.ListPeerEndpoints("endpoints", peerKey)
	if err != nil {
		t.Fatalf("list peer endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	if endpoints[0].Endpoint != "5.6.7.8:51820" {
		t.Errorf("endpoint = %q, want 5.6.7.8:51820", endpoints[0].Endpoint)
	}

	sightings, err := env.Service.ListLocalEndpoints(
		"endpoints",
		testutil.FixedTime.Add(-time.Second),
	)
	if err != nil {
		t.Fatalf("list local endpoints: %v", err)
	}
	if len(sightings) != 1 {
		t.Fatalf("expected 1 sighting, got %d", len(sightings))
	}
	if sightings[0].PeerKey != peerKey {
		t.Errorf("peer key = %q, want %q", sightings[0].PeerKey, peerKey)
	}
}

// TestRecordEndpointAttempt_AdvancesRotationState verifies that a
// recorded attempt is visible in the endpoint catalog, so rotation
// resumes where it left off after a restart.
func TestRecordEndpointAttempt_AdvancesRotationState(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "attempts")

	peerKey := mustGenKey(t)
	testutil.SeedPeers(t, env.Database, "attempts", service.Peer{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	})
	if err := env.Service.RecordLocalEndpoint(
		"attempts",
		peerKey,
		"5.6.7.8:51820",
		testutil.FixedTime,
	); err != nil {
		t.Fatalf("record local endpoint: %v", err)
	}

	if err := env.Service.RecordEndpointAttempt(
		"attempts",
		peerKey,
		"5.6.7.8:51820",
		testutil.FixedTime,
	); err != nil {
		t.Fatalf("record endpoint attempt: %v", err)
	}

	endpoints, err := env.Service.ListPeerEndpoints("attempts", peerKey)
	if err != nil {
		t.Fatalf("list peer endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	if !endpoints[0].LastAttemptedAt.Equal(testutil.FixedTime) {
		t.Errorf("last attempted = %v, want %v", endpoints[0].LastAttemptedAt, testutil.FixedTime)
	}
}

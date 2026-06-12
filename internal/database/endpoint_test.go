package database_test

import (
	"testing"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// setupTwoPeers creates two confirmed peers and returns their public keys
func setupTwoPeers(t *testing.T) (*database.ServerDB, string, string) {
	t.Helper()
	store := setupTestDB(t)

	peerKey := "sighted-peer-key"
	witnessKey := "witness-peer-key"
	if err := createPeerFromInvite(t, store, TestUser1, peerKey); err != nil {
		t.Fatalf("failed to create sighted peer: %v", err)
	}
	if err := createPeerFromInvite(t, store, TestUser2, witnessKey); err != nil {
		t.Fatalf("failed to create witness peer: %v", err)
	}

	return store, peerKey, witnessKey
}

// TestEndpointReportAndQuery tests recording and retrieving a sighting
func TestEndpointReportAndQuery(t *testing.T) {
	store, peerKey, witnessKey := setupTwoPeers(t)

	now := time.Now().Unix()
	err := store.EndpointReport([]server.EndpointSighting{{
		PeerKey:    peerKey,
		WitnessKey: witnessKey,
		Endpoint:   "203.0.113.7:51820",
		Timestamp:  now,
	}})
	expectNoError(t, err, "reporting sighting")

	recent, err := store.EndpointsRecent(now - 60)
	expectNoError(t, err, "querying recent endpoints")

	sightings := recent[peerKey]
	if len(sightings) != 1 {
		t.Fatalf("expected 1 sighting for peer, got %d", len(sightings))
	}
	if sightings[0].Endpoint != "203.0.113.7:51820" {
		t.Errorf("sighting endpoint = %v, want 203.0.113.7:51820", sightings[0].Endpoint)
	}
	if sightings[0].WitnessKey != witnessKey {
		t.Errorf("sighting witness = %v, want %v", sightings[0].WitnessKey, witnessKey)
	}
}

// TestEndpointReportUnknownKeysSkipped tests that unknown keys do not error
func TestEndpointReportUnknownKeysSkipped(t *testing.T) {
	store, _, witnessKey := setupTwoPeers(t)

	now := time.Now().Unix()
	err := store.EndpointReport([]server.EndpointSighting{{
		PeerKey:    "unknown-peer-key",
		WitnessKey: witnessKey,
		Endpoint:   "203.0.113.7:51820",
		Timestamp:  now,
	}})
	expectNoError(t, err, "reporting sighting with unknown peer")

	recent, err := store.EndpointsRecent(now - 60)
	expectNoError(t, err, "querying recent endpoints")
	if len(recent) != 0 {
		t.Errorf("expected no recorded sightings, got %d", len(recent))
	}
}

// TestEndpointsRecentExcludesOld tests the time cutoff
func TestEndpointsRecentExcludesOld(t *testing.T) {
	store, peerKey, witnessKey := setupTwoPeers(t)

	now := time.Now().Unix()
	err := store.EndpointReport([]server.EndpointSighting{{
		PeerKey:    peerKey,
		WitnessKey: witnessKey,
		Endpoint:   "203.0.113.7:51820",
		Timestamp:  now - 3600,
	}})
	expectNoError(t, err, "reporting old sighting")

	recent, err := store.EndpointsRecent(now - 60)
	expectNoError(t, err, "querying recent endpoints")
	if len(recent[peerKey]) != 0 {
		t.Errorf("expected old sighting to be excluded, got %d", len(recent[peerKey]))
	}
}

// TestEndpointsPrune tests deleting old sightings
func TestEndpointsPrune(t *testing.T) {
	store, peerKey, witnessKey := setupTwoPeers(t)

	now := time.Now().Unix()
	err := store.EndpointReport([]server.EndpointSighting{{
		PeerKey:    peerKey,
		WitnessKey: witnessKey,
		Endpoint:   "203.0.113.7:51820",
		Timestamp:  now - 3600,
	}})
	expectNoError(t, err, "reporting old sighting")

	err = store.EndpointsPrune(now - 60)
	expectNoError(t, err, "pruning old sightings")

	all, err := store.EndpointsRecent(0)
	expectNoError(t, err, "querying all endpoints")
	if len(all[peerKey]) != 0 {
		t.Errorf("expected pruned sighting to be gone, got %d", len(all[peerKey]))
	}
}

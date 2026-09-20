package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForEndpoint(
	t *testing.T,
	db *database.DB,
) {
	t.Helper()
	seedDatabaseNetwork(t, db, "epnet")
	for _, p := range []struct {
		name, cidr, pubKey string
	}{
		{"peer-a", "10.0.1.1/32", "pub-a"},
		{"peer-b", "10.0.1.2/32", "pub-b"},
	} {
		testutil.SeedPeerDB(t, db, "epnet", p.name, p.cidr, p.pubKey, false, true, true)
	}
}

func TestInsertEndpointSightings_RoundTrip(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	now := time.Now()
	sightings := []service.EndpointSighting{
		{
			WitnessKey: "pub-a",
			PeerKey:    "pub-b",
			Endpoint:   "192.168.1.100:51820",
			Timestamp:  now,
		},
		{
			WitnessKey: "pub-b",
			PeerKey:    "pub-a",
			Endpoint:   "10.0.0.1:51820",
			Timestamp:  now.Add(time.Minute),
		},
	}

	if err := db.InsertEndpointSightings("epnet", sightings); err != nil {
		t.Fatalf("insert sightings: %v", err)
	}

	endpoints, err := db.GetRecentEndpoints("epnet", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("get recent endpoints: %v", err)
	}

	if len(endpoints) != 2 {
		t.Fatalf("expected 2 peer keys in endpoints map, got %d", len(endpoints))
	}

	t.Logf("endpoints: %+v", endpoints)

	aWitnesses := endpoints["pub-a"]
	if len(aWitnesses) != 1 {
		t.Fatalf("expected 1 witness for pub-a, got %d", len(aWitnesses))
	}
	if aWitnesses[0].Witness != "pub-b" {
		t.Errorf("witness = %q, want pub-b", aWitnesses[0].Witness)
	}
	if aWitnesses[0].Endpoint != "10.0.0.1:51820" {
		t.Errorf("endpoint = %q, want 10.0.0.1:51820", aWitnesses[0].Endpoint)
	}
}

func TestGetRecentEndpoints_IncludesCutoffAndOrdersWitnesses(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)
	cutoff := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	testutil.SeedPeerDB(t, db, "epnet", "peer-c", "10.0.1.3/32", "pub-c", false, true, true)
	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "cutoff:1", Timestamp: cutoff},
		{WitnessKey: "pub-c", PeerKey: "pub-b", Endpoint: "newest:1", Timestamp: cutoff.Add(time.Second)},
		{WitnessKey: "pub-b", PeerKey: "pub-a", Endpoint: "old:1", Timestamp: cutoff.Add(-time.Second)},
	}); err != nil {
		t.Fatal(err)
	}
	// A correction at an equal timestamp replaces the previous observation.
	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "corrected:1", Timestamp: cutoff},
	}); err != nil {
		t.Fatal(err)
	}
	endpoints, err := db.GetRecentEndpoints("epnet", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	witnesses := endpoints["pub-b"]
	if len(endpoints) != 1 || len(witnesses) != 2 {
		t.Fatalf("endpoints = %+v", endpoints)
	}
	if witnesses[0].Witness != "pub-c" || witnesses[0].Endpoint != "newest:1" || !witnesses[0].Timestamp.Equal(cutoff.Add(time.Second)) {
		t.Fatalf("newest witness = %+v", witnesses[0])
	}
	if witnesses[1].Witness != "pub-a" || witnesses[1].Endpoint != "corrected:1" || !witnesses[1].Timestamp.Equal(cutoff) {
		t.Fatalf("cutoff witness = %+v", witnesses[1])
	}
}

func TestInsertEndpointSightings_KeepsLatestPerWitnessAndPeer(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	newer := time.Now()
	older := newer.Add(-time.Minute)
	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "new.ep:1", Timestamp: newer},
	}); err != nil {
		t.Fatalf("insert newer sighting: %v", err)
	}
	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "old.ep:1", Timestamp: older},
	}); err != nil {
		t.Fatalf("insert older sighting: %v", err)
	}

	endpoints, err := db.GetRecentEndpoints("epnet", older.Add(-time.Hour))
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	witnesses := endpoints["pub-b"]
	if len(witnesses) != 1 {
		t.Fatalf("witnesses = %d, want 1", len(witnesses))
	}
	if witnesses[0].Endpoint != "new.ep:1" {
		t.Errorf("endpoint = %q, want new.ep:1", witnesses[0].Endpoint)
	}
}

func TestDeleteEndpointsBefore_PreservesCutoffAndOtherNetworks(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	cutoff := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	oldTime := cutoff.Add(-1 * time.Hour)
	seedDatabaseNetwork(t, db, "other")
	testutil.SeedPeerDB(t, db, "other", "a", "10.0.2.1/32", "pub-a", false, true, true)
	testutil.SeedPeerDB(t, db, "other", "b", "10.0.2.2/32", "pub-b", false, true, true)
	if err := db.InsertEndpointSightings("other", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "other:1", Timestamp: oldTime},
	}); err != nil {
		t.Fatal(err)
	}

	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-b", PeerKey: "pub-a", Endpoint: "old.ep:1", Timestamp: oldTime},
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "recent.ep:1", Timestamp: cutoff},
	}); err != nil {
		t.Fatalf("insert sightings: %v", err)
	}

	if err := db.DeleteEndpointsBefore("epnet", cutoff); err != nil {
		t.Fatalf("delete before: %v", err)
	}

	endpoints, err := db.GetRecentEndpoints("epnet", cutoff.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}

	witnesses := endpoints["pub-b"]
	if len(witnesses) != 1 {
		t.Fatalf("expected 1 witness after prune, got %d", len(witnesses))
	}
	if witnesses[0].Endpoint != "recent.ep:1" {
		t.Errorf("endpoint = %q, want recent.ep:1", witnesses[0].Endpoint)
	}
	if len(endpoints) != 1 || !witnesses[0].Timestamp.Equal(cutoff) {
		t.Fatalf("pruned endpoints = %+v", endpoints)
	}
	other, err := db.GetRecentEndpoints("other", oldTime)
	if err != nil {
		t.Fatal(err)
	}
	if len(other["pub-b"]) != 1 || other["pub-b"][0].Endpoint != "other:1" {
		t.Fatalf("other endpoints = %+v", other)
	}
}

func TestInsertEndpointSightings_MissingNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	err := db.InsertEndpointSightings("missing", nil)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestInsertEndpointSightings_UnknownPeerRejectsBatch(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	now := time.Now()
	err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "valid:1", Timestamp: now},
		{WitnessKey: "pub-a", PeerKey: "ghost", Endpoint: "invalid:1", Timestamp: now},
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	endpoints, err := db.GetRecentEndpoints("epnet", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("expected atomic rollback, got %d endpoint keys", len(endpoints))
	}
}

func TestInsertEndpointSightings_UnknownWitnessRejectsBatch(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "ghost", PeerKey: "pub-b", Endpoint: "ep:1", Timestamp: time.Now()},
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetRecentEndpoints_Empty(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	endpoints, err := db.GetRecentEndpoints("epnet", time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("expected 0 endpoints, got %d", len(endpoints))
	}
}

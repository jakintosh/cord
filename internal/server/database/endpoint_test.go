package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForEndpoint(t *testing.T, db *database.DB) {
	t.Helper()

	var name string = "epnet"
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          name,
				Cidr:          "10.0.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          name + "-i",
				Cidr:          "10.1.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: time.Now(),
		},
		&service.Cidr{
			Name:   "epnet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server-cidr",
			Cidr:     "10.0.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server-cidr",
			Route:     "10.0.0.1/32",
			PublicKey: "pub",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}
	for _, p := range []struct {
		name, cidr, pubKey string
	}{
		{"peer-a", "10.0.1.1/32", "pub-a"},
		{"peer-b", "10.0.1.2/32", "pub-b"},
	} {
		testutil.SeedPeerDB(t, db, "epnet", p.name, p.cidr, p.pubKey, false, true, true)
	}
}

func TestInsertAndGetEndpoints(t *testing.T) {
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

func TestGetRecentEndpoints_SinceFilter(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	oldTime := time.Now().Add(-2 * time.Hour)
	recentTime := time.Now()

	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "old.ep:1", Timestamp: oldTime},
	}); err != nil {
		t.Fatalf("insert old sighting: %v", err)
	}

	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "new.ep:1", Timestamp: recentTime},
	}); err != nil {
		t.Fatalf("insert recent sighting: %v", err)
	}

	endpoints, err := db.GetRecentEndpoints("epnet", recentTime.Add(-time.Hour))
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}

	witnesses := endpoints["pub-b"]
	if len(witnesses) != 1 {
		t.Fatalf("expected 1 witness, got %d", len(witnesses))
	}
	if witnesses[0].Endpoint != "new.ep:1" {
		t.Errorf("endpoint = %q, want new.ep:1", witnesses[0].Endpoint)
	}
}

func TestInsertEndpointSightings_KeepsLatestPerWitnessAndPeer(t *testing.T) {
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

func TestDeleteEndpointsBefore(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	cutoff := time.Now().Add(-1 * time.Hour)
	oldTime := cutoff.Add(-1 * time.Hour)
	recentTime := time.Now()

	if err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "old.ep:1", Timestamp: oldTime},
		{WitnessKey: "pub-a", PeerKey: "pub-b", Endpoint: "recent.ep:1", Timestamp: recentTime},
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
}

func TestInsertEndpointSightings_UnknownPeerSkipped(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForEndpoint(t, db)

	err := db.InsertEndpointSightings("epnet", []service.EndpointSighting{
		{WitnessKey: "pub-a", PeerKey: "ghost", Endpoint: "ep:1", Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatalf("unknown peer should be silently skipped: %v", err)
	}

	endpoints, err := db.GetRecentEndpoints("epnet", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("get recent: %v", err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("expected 0 endpoints after skipped insert, got %d", len(endpoints))
	}
}

func TestEmptyEndpoints(t *testing.T) {
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

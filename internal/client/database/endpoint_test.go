package database_test

import (
	"errors"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"testing"
	"time"
)

func TestRecordLocalEndpoint_MissingPeer(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")

	err := db.RecordLocalEndpoint(
		"testnet",
		"missing-key",
		"1.2.3.4:51820",
		testutil.FixedTime,
	)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecordLocalEndpoint_DoesNotRegress(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	newer := testutil.FixedTime
	older := newer.Add(-time.Hour)
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		newer,
	); err != nil {
		t.Fatalf("record newer endpoint: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		older,
	); err != nil {
		t.Fatalf("record older endpoint: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %#v, want one endpoint", endpoints)
	}
	if !endpoints[0].LocalObservedAt.Equal(newer) {
		t.Errorf("local observed at = %v, want %v", endpoints[0].LocalObservedAt, newer)
	}
}

func TestRecordEndpointAttempt_MissingEndpoint(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}

	err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"missing:51820",
		testutil.FixedTime,
	)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecordEndpointAttempt_DoesNotRegress(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation(
			"alice",
			"alice-key",
			"10.42.0.5/32",
			service.PeerEndpoint{
				Endpoint:         "1.2.3.4:51820",
				ServerObservedAt: testutil.FixedTime,
			},
		),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	newer := testutil.FixedTime
	older := newer.Add(-time.Hour)
	if err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		newer,
	); err != nil {
		t.Fatalf("record newer attempt: %v", err)
	}
	if err := db.RecordEndpointAttempt(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		older,
	); err != nil {
		t.Fatalf("record older attempt: %v", err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatalf("list endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %#v, want one endpoint", endpoints)
	}
	if !endpoints[0].LastAttemptedAt.Equal(newer) {
		t.Errorf("last attempted at = %v, want %v", endpoints[0].LastAttemptedAt, newer)
	}
}

func TestListPeerEndpoints_OrdersLocalBeforeServerObservations(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32",
			service.PeerEndpoint{Endpoint: "older:51820", ServerObservedAt: testutil.FixedTime.Add(-time.Hour)},
			service.PeerEndpoint{Endpoint: "newer:51820", ServerObservedAt: testutil.FixedTime},
		),
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordLocalEndpoint("testnet", "alice-key", "local:51820", testutil.FixedTime.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	endpoints, err := db.ListPeerEndpoints("testnet", "alice-key")
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 3 {
		t.Fatalf("endpoints = %#v, want three", endpoints)
	}
	for i, want := range []string{"local:51820", "newer:51820", "older:51820"} {
		if endpoints[i].Endpoint != want {
			t.Errorf("endpoint %d = %q, want %q", i, endpoints[i].Endpoint, want)
		}
		if !endpoints[i].LastAttemptedAt.IsZero() {
			t.Errorf("endpoint %d has an attempt without recording one", i)
		}
	}
	if !endpoints[0].ServerObservedAt.IsZero() || !endpoints[1].LocalObservedAt.IsZero() {
		t.Fatal("absent observations should round trip as zero time")
	}
}

func TestListLocalEndpointsSince_FiltersByObservationTime(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, db, "testnet")
	if err := db.ApplyNetworkReconciliation("testnet", testutil.NetworkReconciliation(
		peerObservation("alice", "alice-key", "10.42.0.5/32"),
		peerObservation("bob", "bob-key", "10.42.0.6/32"),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"1.2.3.4:51820",
		testutil.FixedTime,
	); err != nil {
		t.Fatalf("record alice endpoint: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"bob-key",
		"5.6.7.8:51820",
		testutil.FixedTime.Add(-time.Hour),
	); err != nil {
		t.Fatalf("record bob endpoint: %v", err)
	}

	sightings, err := db.ListLocalEndpointsSince(
		"testnet",
		testutil.FixedTime.Add(-30*time.Minute),
	)
	if err != nil {
		t.Fatalf("list local endpoints: %v", err)
	}
	if len(sightings) != 1 {
		t.Fatalf("sightings = %d, want 1", len(sightings))
	}
	if sightings[0].PeerKey != "alice-key" ||
		sightings[0].Endpoint != "1.2.3.4:51820" {
		t.Errorf("sighting = %#v, want alice endpoint", sightings[0])
	}
}

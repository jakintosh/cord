package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestListPeers_MissingNetwork(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	_, err := db.ListPeers("missing")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListPeers_PrefersMostRecentLocalEndpoint(
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
				Endpoint:         "server:51820",
				ServerObservedAt: testutil.FixedTime,
			},
		),
	)); err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if err := db.RecordLocalEndpoint(
		"testnet",
		"alice-key",
		"local:51820",
		testutil.FixedTime.Add(-time.Hour),
	); err != nil {
		t.Fatalf("record local endpoint: %v", err)
	}

	peers, err := db.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers = %d, want 1", len(peers))
	}
	if peers[0].Endpoint != "local:51820" {
		t.Errorf("endpoint = %q, want local endpoint", peers[0].Endpoint)
	}
}

package service_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

// testEnv bundles a service with its dependencies for testing.
type testEnv struct {
	svc *service.Service
	db  *database.DB
	wg  *wireguardtest.MockWG
}

var fixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wg := wireguardtest.NewMockWG()

	svc, err := service.New(service.Options{
		Store: db,
		WG:    wg,
		Clock: func() time.Time { return fixedTime },
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &testEnv{svc: svc, db: db, wg: wg}
}

// seedNetwork installs a network via the service and returns it.
func seedNetwork(t *testing.T, svc *service.Service, name string) *service.Network {
	t.Helper()
	nw, err := svc.InstallNetwork(service.Invite{
		NetworkName:    name,
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "server-pub-key",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	})
	if err != nil {
		t.Fatalf("seed network %q: %v", name, err)
	}
	return nw
}

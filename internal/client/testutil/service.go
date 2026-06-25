package testutil

import (
	"log"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

type ServiceEnv struct {
	Database  *database.DB
	WireGuard *wireguardtest.MockWG
	Service   *service.Service
}

func SetupService(
	t *testing.T,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)
	wg := wireguardtest.NewMockWG()

	svc, err := service.New(service.Options{
		Store:  db,
		WG:     wg,
		Clock:  func() time.Time { return FixedTime },
		Logger: log.Default(),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &ServiceEnv{
		Database:  db,
		WireGuard: wg,
		Service:   svc,
	}
}

func SetupServiceWithWG(
	t *testing.T,
	wg wireguard.WG,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)

	svc, err := service.New(service.Options{
		Store:  db,
		WG:     wg,
		Clock:  func() time.Time { return FixedTime },
		Logger: log.Default(),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	var mockWG *wireguardtest.MockWG
	if mw, ok := wg.(*wireguardtest.MockWG); ok {
		mockWG = mw
	}

	return &ServiceEnv{
		Database:  db,
		WireGuard: mockWG,
		Service:   svc,
	}
}

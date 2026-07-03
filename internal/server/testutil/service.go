package testutil

import (
	"log"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

type ServiceEnv struct {
	Database *database.DB
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
	Service  *service.Service
}

func SetupService(
	t *testing.T,
) *ServiceEnv {
	t.Helper()
	return SetupServiceWithClock(t, func() time.Time { return FixedTime })
}

func SetupServiceWithClock(
	t *testing.T,
	clock func() time.Time,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	svc, err := service.New(service.Options{
		Store:     db,
		WireGuard: mgr,
		Clock:     clock,
		Logger:    log.Default(),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &ServiceEnv{
		Database: db,
		Manager:  mgr,
		Backend:  backend,
		Service:  svc,
	}
}

func SetupServiceWithManager(
	t *testing.T,
	mgr *wireguard.Manager,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)

	svc, err := service.New(service.Options{
		Store:     db,
		WireGuard: mgr,
		Clock:     func() time.Time { return FixedTime },
		Logger:    log.Default(),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &ServiceEnv{
		Database: db,
		Manager:  mgr,
		Service:  svc,
	}
}

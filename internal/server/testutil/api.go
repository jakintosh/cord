package testutil

import (
	"log"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

var FixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

type APIEnv struct {
	Database *database.DB
	Service  *service.Service
	Router   http.Handler
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
}

func Setup(
	t *testing.T,
) *APIEnv {
	t.Helper()

	db := SetupDB(t)

	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	svcOpts := service.Options{
		Store:     db,
		WireGuard: mgr,
		Clock:     func() time.Time { return FixedTime },
		Logger:    log.Default(),
	}
	svc, err := service.New(svcOpts)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	apiOpts := admin.Options{
		Service: svc,
		Logger:  log.Default(),
	}
	apiServer, err := admin.New(apiOpts)
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	return &APIEnv{
		Database: db,
		Service:  svc,
		Router:   apiServer.Router(),
		Manager:  mgr,
		Backend:  backend,
	}
}

func (e *APIEnv) SeedNetwork(
	t *testing.T,
) *service.Network {
	return SeedNetwork(t, e.Service)
}

func (e *APIEnv) SeedCIDR(
	t *testing.T,
	network string,
	name string,
	cidr string,
) *service.Cidr {
	t.Helper()

	if err := e.Service.CreateCidr(network, name, cidr); err != nil {
		t.Fatalf("seed cidr %s: %v", name, err)
	}

	c, err := e.Service.GetCidr(network, name)
	if err != nil {
		t.Fatalf("get seeded cidr %s: %v", name, err)
	}
	return c
}

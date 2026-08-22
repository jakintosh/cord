package testutil

import (
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

var FixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

type APIEnv struct {
	Database *database.DB
	Service  *service.Service
	Runtime  *runtime.Runtime
	Router   http.Handler
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
}

func Setup(
	t *testing.T,
) *APIEnv {
	t.Helper()

	env := SetupRuntime(t)

	apiServer, err := admin.New(admin.Options{
		Service: env.Service,
		Runtime: env.Runtime,
		Logger:  logging.Discard(),
	})
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	return &APIEnv{
		Database: env.Database,
		Service:  env.Service,
		Runtime:  env.Runtime,
		Router:   apiServer.Router(),
		Manager:  env.Manager,
		Backend:  env.Backend,
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

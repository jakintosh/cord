package testutil

import (
	"context"
	"log"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

var FixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

type APIEnv struct {
	Database *database.DB
	Service  *service.Service
	Router   http.Handler
}

func Setup(
	t *testing.T,
) *APIEnv {
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

	apiServer, err := api.New(api.Options{
		Service: svc,
		Logger:  log.Default(),
	})
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	return &APIEnv{
		Database: db,
		Service:  svc,
		Router:   apiServer.Router(),
	}
}

func (e *APIEnv) SeedNetwork(
	t *testing.T,
	name string,
) *service.Network {
	return SeedNetworkWithName(t, e.Service, name)
}

func (e *APIEnv) SeedEnabledNetwork(
	t *testing.T,
	name string,
) *service.Network {
	t.Helper()

	nw := e.SeedNetwork(t, name)
	if err := e.Service.EnableNetwork(context.Background(), name); err != nil {
		t.Fatalf("enable network %q: %v", name, err)
	}
	return nw
}

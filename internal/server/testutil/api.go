package testutil

import (
	"log"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

type APIEnv struct {
	DB      *database.DB
	Service *service.Service
	Router  http.Handler
	WG      *wireguardtest.MockWG
}

func Setup(t *testing.T) *APIEnv {
	t.Helper()

	db := SetupDB(t)

	wg := wireguardtest.NewMockWG()

	svcOpts := service.Options{
		Store:  db,
		WG:     wg,
		Clock:  func() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) },
		Logger: log.Default(),
	}
	svc, err := service.New(svcOpts)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	apiOpts := api.Options{
		Service: svc,
		Logger:  log.Default(),
	}
	apiServer, err := api.New(apiOpts)
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	return &APIEnv{
		DB:      db,
		Service: svc,
		Router:  apiServer.Router(),
		WG:      wg,
	}
}

func (e *APIEnv) SeedNetwork(t *testing.T) *service.Network {
	t.Helper()

	net, err := e.Service.CreateNetwork(service.Network{
		Name:             "testnet",
		RootCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "192.168.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if err != nil {
		t.Fatalf("seed network: %v", err)
	}
	return net
}

func (e *APIEnv) SeedCIDR(t *testing.T, network string, name string, cidr string) *service.Cidr {
	t.Helper()

	if err := e.Service.AddCidr(network, service.CreateCidrRequest{
		Name: name,
		Cidr: cidr,
	}); err != nil {
		t.Fatalf("seed cidr %s: %v", name, err)
	}

	c, err := e.Service.GetCidr(network, name)
	if err != nil {
		t.Fatalf("get seeded cidr %s: %v", name, err)
	}
	return c
}

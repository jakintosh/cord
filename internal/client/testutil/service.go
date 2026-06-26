package testutil

import (
	"log"
	"net/http"
	"net/http/httptest"
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
	Server    *httptest.Server
}

func SetupService(
	t *testing.T,
) *ServiceEnv {
	t.Helper()
	return SetupServiceWithServer(t, nil)
}

func SetupServiceWithServer(
	t *testing.T,
	handler http.Handler,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)
	wg := wireguardtest.NewMockWG()

	var httpClient *http.Client
	var server *httptest.Server
	if handler != nil {
		server = httptest.NewServer(handler)
		httpClient = server.Client()
	}

	svc, err := service.New(service.Options{
		Store:        db,
		WG:           wg,
		Clock:        func() time.Time { return FixedTime },
		Logger:       log.Default(),
		HTTPClient:   httpClient,
		SyncInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &ServiceEnv{
		Database:  db,
		WireGuard: wg,
		Service:   svc,
		Server:    server,
	}
}

func SetupServiceWithWG(
	t *testing.T,
	wg wireguard.WG,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)

	svc, err := service.New(service.Options{
		Store:        db,
		WG:           wg,
		Clock:        func() time.Time { return FixedTime },
		Logger:       log.Default(),
		SyncInterval: 30 * time.Second,
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

package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

type ServiceEnv struct {
	Database *database.DB
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
	Service  *service.Service
	Server   *httptest.Server
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
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	var server *httptest.Server
	// The sync timer fires immediately on start; when no test server
	// backs the tunnel address the call must fail fast instead of
	// waiting out the default dial timeout.
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	if handler != nil {
		server = httptest.NewServer(handler)
		httpClient = server.Client()
	}

	svc, err := service.New(service.Options{
		Store:      db,
		WireGuard:  mgr,
		Clock:      func() time.Time { return FixedTime },
		Logger:     logging.Discard(),
		HTTPClient: httpClient,

		SyncInterval:   30 * time.Second,
		ScanInterval:   30 * time.Second,
		ReportInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &ServiceEnv{
		Database: db,
		Manager:  mgr,
		Backend:  backend,
		Service:  svc,
		Server:   server,
	}
}

func SetupServiceWithManager(
	t *testing.T,
	mgr *wireguard.Manager,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)

	svc, err := service.New(service.Options{
		Store:      db,
		WireGuard:  mgr,
		Clock:      func() time.Time { return FixedTime },
		Logger:     logging.Discard(),
		HTTPClient: &http.Client{Timeout: 100 * time.Millisecond},

		SyncInterval:   30 * time.Second,
		ScanInterval:   30 * time.Second,
		ReportInterval: 30 * time.Second,
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

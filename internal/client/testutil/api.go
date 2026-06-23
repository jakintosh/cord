package testutil

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

type mockWG struct {
	mu     sync.Mutex
	seq    int
	devErr error
}

func (m *mockWG) GenerateKey() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return fmt.Sprintf("mock-priv-key-%d", m.seq), nil
}

func (m *mockWG) PublicKey(privateKey string) (string, error) {
	return privateKey + "-pub", nil
}

func (m *mockWG) NewDevice(name, privateKey, address string, port uint16) (service.WGDevice, error) {
	if m.devErr != nil {
		return nil, m.devErr
	}
	return &mockDevice{}, nil
}

func (m *mockWG) RemoveDevice(name string) error {
	return nil
}

type mockDevice struct{}

func (d *mockDevice) ApplyPeers(peers []service.WGPeer) error { return nil }
func (d *mockDevice) Up() error                               { return nil }
func (d *mockDevice) Down(remove bool) error                  { return nil }
func (d *mockDevice) DeviceName() string                      { return "mock" }

type APIEnv struct {
	DB      *database.DB
	Service *service.Service
	Router  http.Handler
}

var fixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

func Setup(t *testing.T) *APIEnv {
	t.Helper()

	db := SetupDB(t)

	wg := &mockWG{}

	svc, err := service.New(service.Options{
		Store:  db,
		WG:     wg,
		Clock:  func() time.Time { return fixedTime },
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
		DB:      db,
		Service: svc,
		Router:  apiServer.Router(),
	}
}

func (e *APIEnv) SeedNetwork(t *testing.T, name string) *service.Network {
	t.Helper()

	nw, err := e.Service.InstallNetwork(service.Invite{
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

func (e *APIEnv) SeedEnabledNetwork(t *testing.T, name string) *service.Network {
	t.Helper()

	nw := e.SeedNetwork(t, name)
	if err := e.Service.EnableNetwork(context.Background(), name); err != nil {
		t.Fatalf("enable network %q: %v", name, err)
	}
	return nw
}

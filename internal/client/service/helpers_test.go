package service_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

// mockWG is a test double that generates predictable keys and creates
// mock devices that track their calls.
type mockWG struct {
	mu      sync.Mutex
	keySeq  int
	devices map[string]*mockDevice
	newErr  error
}

func newMockWG() *mockWG {
	return &mockWG{
		devices: make(map[string]*mockDevice),
	}
}

func (m *mockWG) GenerateKey() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keySeq++
	return fmt.Sprintf("mock-priv-key-%d", m.keySeq), nil
}

func (m *mockWG) PublicKey(privateKey string) (string, error) {
	return privateKey + "-pub", nil
}

func (m *mockWG) NewDevice(name, privateKey, address string, port uint16) (service.WGDevice, error) {
	if m.newErr != nil {
		return nil, m.newErr
	}
	d := &mockDevice{name: name}
	m.mu.Lock()
	m.devices[name] = d
	m.mu.Unlock()
	return d, nil
}

func (m *mockWG) RemoveDevice(name string) error {
	m.mu.Lock()
	delete(m.devices, name)
	m.mu.Unlock()
	return nil
}

// mockDevice is a test double for WGDevice that records calls.
type mockDevice struct {
	mu         sync.Mutex
	name       string
	peers      []service.WGPeer
	upCalls    int
	downCalls  int
	lastDownRm bool
	upErr      error
	downErr    error
	applyErr   error
}

func (d *mockDevice) ApplyPeers(peers []service.WGPeer) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.peers = peers
	return d.applyErr
}

func (d *mockDevice) Up() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upCalls++
	return d.upErr
}

func (d *mockDevice) Down(remove bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.downCalls++
	d.lastDownRm = remove
	return d.downErr
}

func (d *mockDevice) DeviceName() string {
	return d.name
}

func (d *mockDevice) AppliedPeers() []service.WGPeer {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.peers
}

func (d *mockDevice) UpCalls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.upCalls
}

func (d *mockDevice) DownCalls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.downCalls
}

func (d *mockDevice) LastDownRemove() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastDownRm
}

// testEnv bundles a service with its dependencies for testing.
type testEnv struct {
	svc *service.Service
	db  *database.DB
	wg  *mockWG
}

var fixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wg := newMockWG()

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

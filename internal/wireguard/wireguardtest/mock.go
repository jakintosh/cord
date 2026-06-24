// Package wireguardtest provides test doubles for the wireguard
// package's WG and WGDevice interfaces.
package wireguardtest

import (
	"fmt"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// MockWG is a test double for wireguard.WG that generates
// predictable keys and creates MockDevice instances.
type MockWG struct {
	mu      sync.Mutex
	keySeq  int
	Devices map[string]*MockDevice
	NewErr  error
}

// NewMockWG returns a ready-to-use MockWG.
func NewMockWG() *MockWG {
	return &MockWG{
		Devices: make(map[string]*MockDevice),
	}
}

func (m *MockWG) GenerateKey() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keySeq++
	return fmt.Sprintf("mock-priv-key-%d", m.keySeq), nil
}

func (m *MockWG) PublicKey(privateKey string) (string, error) {
	return privateKey + "-pub", nil
}

func (m *MockWG) NewDevice(name, privateKey, address string, port uint16) (wireguard.WGDevice, error) {
	if m.NewErr != nil {
		return nil, m.NewErr
	}
	d := &MockDevice{Name: name}
	m.mu.Lock()
	m.Devices[name] = d
	m.mu.Unlock()
	return d, nil
}

func (m *MockWG) RemoveDevice(name string) error {
	return nil
}

// MockDevice is a test double for wireguard.WGDevice that records
// calls for assertions.
type MockDevice struct {
	mu        sync.Mutex
	Name      string
	Peers     []wireguard.WGPeer
	UpCalls   int
	DownCalls int
	UpErr     error
	DownErr   error
	ApplyErr  error
}

func (d *MockDevice) ApplyPeers(peers []wireguard.WGPeer) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Peers = peers
	return d.ApplyErr
}

func (d *MockDevice) Up() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.UpCalls++
	return d.UpErr
}

func (d *MockDevice) Down() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.DownCalls++
	return d.DownErr
}

func (d *MockDevice) DeviceName() string {
	return d.Name
}

func (d *MockDevice) WaitForHandshake(pubKey string, timeout time.Duration, onStatus func(wireguard.PeerStatus)) error {
	return nil
}

// AppliedPeers returns the last set of peers applied to this device.
func (d *MockDevice) AppliedPeers() []wireguard.WGPeer {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Peers
}

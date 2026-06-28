// Package wireguardtest provides test doubles for the wireguard
// package's WG and WGDevice interfaces.
package wireguardtest

import (
	"fmt"
	"net"
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

func (m *MockWG) GenerateKey() (
	string,
	error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keySeq++
	return fmt.Sprintf("mock-priv-key-%d", m.keySeq), nil
}

func (m *MockWG) PublicKey(
	privateKey string,
) (
	string,
	error,
) {
	return privateKey + "-pub", nil
}

func (m *MockWG) NewDevice(
	name string,
	privateKey string,
	address net.IPNet,
	port uint16,
) (
	wireguard.WGDevice,
	error,
) {
	if m.NewErr != nil {
		return nil, m.NewErr
	}
	d := &MockDevice{
		Name:       name,
		PrivateKey: privateKey,
		Address:    address.String(),
		Port:       port,
	}
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
	mu              sync.Mutex
	Name            string
	PrivateKey      string
	Address         string
	Port            uint16
	Peers           []wireguard.WGPeer
	UpCalls         int
	DownCalls       int
	ApplyCalls      int
	IsUp            bool
	UpErr           error
	DownErr         error
	ApplyErr        error
	StatusErr       error
	WaitCalls       int
	EndpointUpdates []EndpointUpdate
	UpdateErr       error
}

// EndpointUpdate records a call to UpdateEndpoint.
type EndpointUpdate struct {
	PubKey   string
	Endpoint string
}

func (d *MockDevice) ApplyPeers(
	peers []wireguard.WGPeer,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ApplyCalls++
	if !d.IsUp {
		return wireguard.ErrDeviceNotUp
	}
	d.Peers = peers
	return d.ApplyErr
}

func (d *MockDevice) UpdateEndpoint(
	pubKey string,
	endpoint string,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.UpdateErr != nil {
		return d.UpdateErr
	}
	d.EndpointUpdates = append(d.EndpointUpdates, EndpointUpdate{
		PubKey:   pubKey,
		Endpoint: endpoint,
	})
	return nil
}

func (d *MockDevice) Up() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.UpCalls++
	if d.UpErr == nil {
		d.IsUp = true
	}
	return d.UpErr
}

func (d *MockDevice) Down() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.DownCalls++
	if d.DownErr == nil {
		d.IsUp = false
	}
	return d.DownErr
}

func (d *MockDevice) DeviceName() string {
	return d.Name
}

func (d *MockDevice) WaitForHandshake(
	pubKey string,
	timeout time.Duration,
	onStatus func(wireguard.PeerStatus),
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.WaitCalls++
	return nil
}

func (d *MockDevice) Status() (
	[]wireguard.PeerStatus,
	error,
) {
	return nil, d.StatusErr
}

// AppliedPeers returns the last set of peers applied to this device.
func (d *MockDevice) AppliedPeers() []wireguard.WGPeer {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Peers
}

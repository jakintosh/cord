// Package wireguardtest provides test doubles for the wireguard package.
package wireguardtest

import (
	"net"
	"sort"
	"sync"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// applyCall records one ApplyPeers call for batch tracking.
type applyCall struct {
	Ops []wireguard.PeerOp
}

// MockDevice is a BackendDevice handle for testing. Each MockDevice
// holds its own peer map, so tests can inspect per-device state
// independently.
type MockDevice struct {
	mu            sync.Mutex
	name          string
	peers         map[string]wireguard.PeerStatus
	PeersCalls    int
	ApplyCalls    []applyCall
	CloseCalls    int
	PeersErr      error
	ApplyPeersErr error
	CloseErr      error
}

func (d *MockDevice) Peers() (
	[]wireguard.PeerStatus,
	error,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.PeersCalls++
	if d.PeersErr != nil {
		return nil, d.PeersErr
	}
	return d.peersListLocked(), nil
}

func (d *MockDevice) ApplyPeers(
	ops []wireguard.PeerOp,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ApplyPeersErr != nil {
		return d.ApplyPeersErr
	}
	call := applyCall{Ops: make([]wireguard.PeerOp, len(ops))}
	copy(call.Ops, ops)
	d.ApplyCalls = append(d.ApplyCalls, call)
	for _, op := range ops {
		key := op.Config.PublicKey.String()
		if op.Remove {
			delete(d.peers, key)
		} else {
			ps := wireguard.PeerStatus{
				PublicKey:           op.Config.PublicKey,
				AllowedIPs:          copyIPNets(op.Config.AllowedIPs),
				Endpoint:            copyUDPAddr(op.Config.Endpoint),
				PersistentKeepalive: op.Config.PersistentKeepalive,
			}
			d.peers[key] = ps
		}
	}
	return nil
}

// AppliedOps returns all peer operations applied to this device.
func (d *MockDevice) AppliedOps() []wireguard.PeerOp {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []wireguard.PeerOp
	for _, call := range d.ApplyCalls {
		out = append(out, call.Ops...)
	}
	return out
}

// LastAppliedOps returns the most recent batch of peer operations.
func (d *MockDevice) LastAppliedOps() []wireguard.PeerOp {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.ApplyCalls) == 0 {
		return nil
	}
	last := d.ApplyCalls[len(d.ApplyCalls)-1]
	out := make([]wireguard.PeerOp, len(last.Ops))
	copy(out, last.Ops)
	return out
}

func (d *MockDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.CloseCalls++
	if d.CloseErr != nil {
		return d.CloseErr
	}
	return nil
}

func (d *MockDevice) peersListLocked() []wireguard.PeerStatus {
	out := make([]wireguard.PeerStatus, 0, len(d.peers))
	for _, p := range d.peers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PublicKey.String() < out[j].PublicKey.String()
	})
	return out
}

// SetPeers replaces the mock's observed peer state for this device.
func (d *MockDevice) SetPeers(
	peers ...wireguard.PeerStatus,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	devPeers := make(map[string]wireguard.PeerStatus, len(peers))
	for _, p := range peers {
		devPeers[p.PublicKey.String()] = p
	}
	d.peers = devPeers
}

// MockBackend implements wireguard.Backend for testing. It creates
// MockDevice handles that record all calls.
type MockBackend struct {
	mu          sync.Mutex
	devices     map[string]*MockDevice
	CreateCalls []wireguard.DeviceConfig
	CreateErr   error
}

// NewMockBackend returns a ready-to-use MockBackend.
func NewMockBackend() *MockBackend {
	return &MockBackend{
		devices: make(map[string]*MockDevice),
	}
}

func (b *MockBackend) CreateDevice(
	cfg wireguard.DeviceConfig,
) (
	wireguard.BackendDevice,
	error,
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.CreateErr != nil {
		return nil, b.CreateErr
	}
	b.CreateCalls = append(b.CreateCalls, cfg)
	dev := &MockDevice{
		name:  cfg.Name,
		peers: make(map[string]wireguard.PeerStatus),
	}
	b.devices[cfg.Name] = dev
	return dev, nil
}

// Device returns the MockDevice for the given name, or nil.
func (b *MockBackend) Device(
	name string,
) *MockDevice {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.devices[name]
}

// AppliedOpsFor returns all peer operations applied for a specific device.
func (b *MockBackend) AppliedOpsFor(
	name string,
) []wireguard.PeerOp {
	b.mu.Lock()
	dev, ok := b.devices[name]
	b.mu.Unlock()
	if !ok {
		return nil
	}
	return dev.AppliedOps()
}

// LastAppliedOpsFor returns peer operations from the most recent ApplyPeers
// call for a specific device.
func (b *MockBackend) LastAppliedOpsFor(
	name string,
) []wireguard.PeerOp {
	b.mu.Lock()
	dev, ok := b.devices[name]
	b.mu.Unlock()
	if !ok {
		return nil
	}
	return dev.LastAppliedOps()
}

// Reset clears all recorded calls, peer state, and injected errors.
func (b *MockBackend) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.CreateCalls = nil
	b.CreateErr = nil
	b.devices = make(map[string]*MockDevice)
}

func copyIPNets(
	ips []net.IPNet,
) []net.IPNet {
	if ips == nil {
		return nil
	}
	out := make([]net.IPNet, len(ips))
	for i, ipNet := range ips {
		out[i] = net.IPNet{
			IP:   append(net.IP(nil), ipNet.IP...),
			Mask: append(net.IPMask(nil), ipNet.Mask...),
		}
	}
	return out
}

func copyUDPAddr(
	addr *net.UDPAddr,
) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	out := *addr
	out.IP = append(net.IP(nil), addr.IP...)
	return &out
}

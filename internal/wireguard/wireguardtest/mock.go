// Package wireguardtest provides test doubles for the wireguard package.
package wireguardtest

import (
	"net"
	"sort"
	"sync"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// UpConfig records the parameters passed to Backend.Up for inspection
// in tests.
type UpConfig struct {
	Name       string
	PrivateKey wgtypes.Key
	Address    net.IPNet
	ListenPort int
	MTU        int
	NoRoutes   bool
}

// MockBackend implements wireguard.Backend for testing. It records all
// calls and maintains in-memory peer state so that deviations from
// Peers() calls reflect previously applied operations.
type MockBackend struct {
	mu sync.Mutex

	// Per-device state: configs passed to Up, indexed by device name.
	UpConfigs map[string]UpConfig

	// Call counters / records.
	UpCalls     []string
	PeersCalls  []string
	DownCalls   []string
	DeleteCalls []string
	ApplyCalls  []ApplyCall

	// Per-device peer state, updated by ApplyPeerOperations.
	peers map[string]map[string]wireguard.Peer

	// Controlled responses.
	PeersErr error

	// Error injection.
	UpErr     error
	DownErr   error
	DeleteErr error
	ApplyErr  error
}

// ApplyCall records one invocation of ApplyPeerOperations.
type ApplyCall struct {
	Name       string
	Operations []wireguard.PeerOperation
}

// NewMockBackend returns a ready-to-use MockBackend.
func NewMockBackend() *MockBackend {
	return &MockBackend{
		UpConfigs: make(map[string]UpConfig),
		peers:     make(map[string]map[string]wireguard.Peer),
	}
}

func (b *MockBackend) ensureLocked() {
	if b.UpConfigs == nil {
		b.UpConfigs = make(map[string]UpConfig)
	}
	if b.peers == nil {
		b.peers = make(map[string]map[string]wireguard.Peer)
	}
}

func (b *MockBackend) Up(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	listenPort int,
	mtu int,
	noRoutes bool,
) error {
	b.mu.Lock()
	b.ensureLocked()
	defer b.mu.Unlock()
	if b.UpErr != nil {
		return b.UpErr
	}
	b.UpCalls = append(b.UpCalls, name)
	b.UpConfigs[name] = UpConfig{
		Name:       name,
		PrivateKey: privateKey,
		Address:    address,
		ListenPort: listenPort,
		MTU:        mtu,
		NoRoutes:   noRoutes,
	}
	return nil
}

func (b *MockBackend) Down(
	name string,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.DownErr != nil {
		return b.DownErr
	}
	b.DownCalls = append(b.DownCalls, name)
	return nil
}

func (b *MockBackend) Delete(
	name string,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.DeleteErr != nil {
		return b.DeleteErr
	}
	b.DeleteCalls = append(b.DeleteCalls, name)
	return nil
}

func (b *MockBackend) GetPeers(
	name string,
) (
	[]wireguard.Peer,
	error,
) {
	b.mu.Lock()
	b.ensureLocked()
	defer b.mu.Unlock()
	b.PeersCalls = append(b.PeersCalls, name)
	if b.PeersErr != nil {
		return nil, b.PeersErr
	}
	return b.peersForLocked(name), nil
}

func (b *MockBackend) ModifyPeers(
	name string,
	operations []wireguard.PeerOperation,
) error {
	b.mu.Lock()
	b.ensureLocked()
	defer b.mu.Unlock()
	if b.ApplyErr != nil {
		return b.ApplyErr
	}
	ops := copyPeerOperations(operations)
	b.ApplyCalls = append(b.ApplyCalls, ApplyCall{
		Name:       name,
		Operations: ops,
	})

	// Update in-memory peer state so Peers() reflects applied ops.
	if _, ok := b.peers[name]; !ok {
		b.peers[name] = make(map[string]wireguard.Peer)
	}
	for _, op := range ops {
		key := op.Peer.PublicKey.String()
		switch op.Type {
		case wireguard.PeerAdd:
			b.peers[name][key] = copyPeer(op.Peer)
		case wireguard.PeerRemove:
			delete(b.peers[name], key)
		case wireguard.PeerUpdate:
			if existing, ok := b.peers[name][key]; ok {
				if op.UpdateAllowedIPs {
					existing.AllowedIPs = copyIPNets(op.Peer.AllowedIPs)
				}
				if op.UpdateEndpoint {
					existing.Endpoint = copyUDPAddr(op.Peer.Endpoint)
				}
				if op.UpdateKeepalive {
					existing.PersistentKeepalive = op.Peer.PersistentKeepalive
				}
				b.peers[name][key] = existing
			}
		}
	}
	return nil
}

// AppliedOps returns all peer operations applied across all devices.
func (b *MockBackend) AppliedOps() []wireguard.PeerOperation {
	b.mu.Lock()
	defer b.mu.Unlock()
	var ops []wireguard.PeerOperation
	for _, c := range b.ApplyCalls {
		ops = append(ops, c.Operations...)
	}
	return copyPeerOperations(ops)
}

// AppliedOpsFor returns peer operations applied for a specific device.
func (b *MockBackend) AppliedOpsFor(
	name string,
) []wireguard.PeerOperation {
	b.mu.Lock()
	defer b.mu.Unlock()
	var ops []wireguard.PeerOperation
	for _, c := range b.ApplyCalls {
		if c.Name == name {
			ops = append(ops, c.Operations...)
		}
	}
	return copyPeerOperations(ops)
}

// LastAppliedOpsFor returns peer operations from the most recent
// ApplyPeerOperations call for a specific device.
func (b *MockBackend) LastAppliedOpsFor(
	name string,
) []wireguard.PeerOperation {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.ApplyCalls) - 1; i >= 0; i-- {
		if b.ApplyCalls[i].Name == name {
			return copyPeerOperations(b.ApplyCalls[i].Operations)
		}
	}
	return nil
}

// ApplyCountFor returns the number of ApplyPeerOperations calls for a device.
func (b *MockBackend) ApplyCountFor(
	name string,
) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, c := range b.ApplyCalls {
		if c.Name == name {
			n++
		}
	}
	return n
}

// UpCount returns the number of Up calls for a device.
func (b *MockBackend) UpCount(
	name string,
) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, d := range b.UpCalls {
		if d == name {
			n++
		}
	}
	return n
}

// PeersCount returns the number of Peers calls for a device.
func (b *MockBackend) PeersCount(
	name string,
) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, d := range b.PeersCalls {
		if d == name {
			n++
		}
	}
	return n
}

// DownCount returns the number of Down calls for a device.
func (b *MockBackend) DownCount(
	name string,
) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, d := range b.DownCalls {
		if d == name {
			n++
		}
	}
	return n
}

// DeleteCount returns the number of Delete calls for a device.
func (b *MockBackend) DeleteCount(
	name string,
) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, d := range b.DeleteCalls {
		if d == name {
			n++
		}
	}
	return n
}

// SetPeers replaces the mock's observed peer state for a device.
func (b *MockBackend) SetPeers(
	name string,
	peers ...wireguard.Peer,
) {
	b.mu.Lock()
	b.ensureLocked()
	defer b.mu.Unlock()

	devPeers := make(map[string]wireguard.Peer, len(peers))
	for _, p := range peers {
		devPeers[p.PublicKey.String()] = copyPeer(p)
	}
	b.peers[name] = devPeers
}

// PeersFor returns the mock's current observed peer state for a device.
func (b *MockBackend) PeersFor(
	name string,
) []wireguard.Peer {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.peersForLocked(name)
}

// Reset clears all recorded calls, peer state, and injected errors.
func (b *MockBackend) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.UpConfigs = make(map[string]UpConfig)
	b.UpCalls = nil
	b.PeersCalls = nil
	b.DownCalls = nil
	b.DeleteCalls = nil
	b.ApplyCalls = nil
	b.peers = make(map[string]map[string]wireguard.Peer)
	b.PeersErr = nil
	b.UpErr = nil
	b.DownErr = nil
	b.DeleteErr = nil
	b.ApplyErr = nil
}

func (b *MockBackend) peersForLocked(
	name string,
) []wireguard.Peer {
	devPeers := b.peers[name]
	status := make([]wireguard.Peer, 0, len(devPeers))
	for _, p := range devPeers {
		status = append(status, copyPeer(p))
	}
	sort.Slice(status, func(i, j int) bool {
		return status[i].PublicKey.String() < status[j].PublicKey.String()
	})
	return status
}

func copyPeerOperations(
	ops []wireguard.PeerOperation,
) []wireguard.PeerOperation {
	if ops == nil {
		return nil
	}
	out := make([]wireguard.PeerOperation, len(ops))
	for i, op := range ops {
		out[i] = op
		out[i].Peer = copyPeer(op.Peer)
	}
	return out
}

func copyPeer(
	p wireguard.Peer,
) wireguard.Peer {
	p.AllowedIPs = copyIPNets(p.AllowedIPs)
	p.Endpoint = copyUDPAddr(p.Endpoint)
	return p
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

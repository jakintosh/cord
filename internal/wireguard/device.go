package wireguard

import (
	"fmt"
	"net"
	"sync"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// DeviceConfig configures a new WireGuard device. CreateDevice
// creates the interface, configures it, and brings it up in one
// step — the "created but not up" state does not exist.
type DeviceConfig struct {
	Name       string
	PrivateKey string
	Route      net.IPNet
	ListenPort uint16
	MTU        int // 0 uses the default
}

// Device is a live WireGuard network device backed by a WgDevice.
// All methods are safe for concurrent use; the Device serializes access
// with a single mutex held across backend calls.
type Device struct {
	name    string
	mu      sync.Mutex
	backend WgDevice
	desired map[wgtypes.Key]Peer
}

func newDevice(
	name string,
	backend WgDevice,
) *Device {
	return &Device{
		name:    name,
		backend: backend,
		desired: make(map[wgtypes.Key]Peer),
	}
}

func (d *Device) Name() string {
	return d.name
}

// SetPeers replaces the desired peer set and reconciles it against
// the live WireGuard state. Each PeerConfig is parsed and validated
// before any changes are applied. The mutex is held across the entire
// observe→plan→apply cycle so that concurrent calls are serialized.
func (d *Device) SetPeers(
	configs ...PeerConfig,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	peers := make([]Peer, len(configs))
	for i, cfg := range configs {
		var err error
		peers[i], err = cfg.Parse()
		if err != nil {
			return fmt.Errorf("wireguard: peer config %d: %w", i, err)
		}
	}

	d.desired = make(map[wgtypes.Key]Peer, len(peers))
	for _, p := range peers {
		d.desired[p.PublicKey] = p
	}

	observed, err := d.backend.Peers()
	if err != nil {
		return fmt.Errorf("wireguard: reconcile observe: %w", err)
	}
	ops := planPeerReconciliation(peers, observed)
	if len(ops) == 0 {
		return nil
	}
	return d.backend.ApplyPeers(ops)
}

// Peers returns the live peer state from the WireGuard device.
func (d *Device) Peers() (
	[]PeerStatus,
	error,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.backend.Peers()
}

// SetPeerEndpoint updates the endpoint for a single existing peer,
// bypassing the normal reconciliation flow. It updates the desired
// entry's endpoint and applies a targeted op with the full peer config
// so that routing is preserved.
func (d *Device) SetPeerEndpoint(
	pubKey string,
	endpoint string,
) error {
	key, err := parseKey(pubKey)
	if err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	peer, ok := d.desired[key]
	if !ok {
		return fmt.Errorf("wireguard: update endpoint: peer %s not found", pubKey)
	}
	peer.Endpoint = addr
	d.desired[key] = peer

	return d.backend.ApplyPeers([]PeerOp{{Target: peer}})
}

// Close brings the device down and removes it.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.backend.Close()
}

// ValidateDeviceName checks that a device name fits within the kernel
// interface name length limit.
func ValidateDeviceName(
	name string,
) error {
	if len(name) > maxInterfaceNameBytes {
		return fmt.Errorf("wireguard: device name %q exceeds %d byte limit", name, maxInterfaceNameBytes)
	}
	return nil
}

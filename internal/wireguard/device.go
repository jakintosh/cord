package wireguard

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

// EndpointPolicy controls how Cord manages a peer's endpoint.
type EndpointPolicy int

const (
	EndpointDynamic   EndpointPolicy = iota // cord never touches endpoint; learned from handshakes (default)
	EndpointBootstrap                       // set only on initial add
	EndpointFixed                           // always reconciled
)

// PeerConfig is a desired WireGuard peer. It carries only
// configuration fields — runtime state like handshake time and byte
// counters lives in PeerStatus.
type PeerConfig struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	EndpointPolicy      EndpointPolicy
	PersistentKeepalive time.Duration
}

// NewPeerConfig parses string representations into a PeerConfig.
// endpoint may be empty. keepaliveSec is seconds; 0 means no keepalive.
func NewPeerConfig(
	publicKey string,
	allowedIPs []string,
	endpoint string,
	keepaliveSec int,
	policy EndpointPolicy,
) (
	PeerConfig,
	error,
) {
	key, err := parseKey(publicKey)
	if err != nil {
		return PeerConfig{}, fmt.Errorf("public key %q: %w", publicKey, err)
	}

	var ips []net.IPNet
	for _, cidr := range allowedIPs {
		ipNet, err := netaddr.ParseInterface(cidr)
		if err != nil {
			return PeerConfig{}, fmt.Errorf("allowed-ip %q: %w", cidr, err)
		}
		ips = append(ips, ipNet)
	}

	var ep *net.UDPAddr
	if endpoint != "" {
		ep, err = net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			return PeerConfig{}, fmt.Errorf("endpoint %q: %w", endpoint, err)
		}
	}

	return PeerConfig{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      policy,
		PersistentKeepalive: time.Duration(keepaliveSec) * time.Second,
	}, nil
}

// PeerStatus is the observed live state of a WireGuard peer returned
// by the backend. It includes runtime fields that PeerConfig does not.
type PeerStatus struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
	LastHandshake       time.Time
	ReceiveBytes        int64
	TransmitBytes       int64
}

// DeviceConfig configures a new WireGuard device. CreateDevice
// creates the interface, configures it, and brings it up in one
// step — the "created but not up" state does not exist.
type DeviceConfig struct {
	Name       string
	PrivateKey string
	Address    net.IPNet
	ListenPort uint16
	MTU        int // 0 uses the default
}

// Device is a live WireGuard network device backed by a BackendDevice.
// All methods are safe for concurrent use; the Device serializes access
// with a single mutex held across backend calls.
type Device struct {
	name    string
	mu      sync.Mutex
	backend BackendDevice
	desired map[wgtypes.Key]PeerConfig
}

func newDevice(
	name string,
	backend BackendDevice,
) *Device {
	return &Device{
		name:    name,
		backend: backend,
		desired: make(map[wgtypes.Key]PeerConfig),
	}
}

func (d *Device) Name() string {
	return d.name
}

// SetPeers replaces the desired peer set and reconciles it against
// the live WireGuard state. The mutex is held across the entire
// observe→plan→apply cycle so that concurrent calls are serialized.
func (d *Device) SetPeers(
	peers ...PeerConfig,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.desired = make(map[wgtypes.Key]PeerConfig, len(peers))
	desiredSlice := make([]PeerConfig, len(peers))
	for i, p := range peers {
		d.desired[p.PublicKey] = p
		desiredSlice[i] = p
	}

	observed, err := d.backend.Peers()
	if err != nil {
		return fmt.Errorf("wireguard: reconcile observe: %w", err)
	}
	ops := planPeerReconciliation(desiredSlice, observed)
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

	return d.backend.ApplyPeers([]PeerOp{{Config: peer}})
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

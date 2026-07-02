package wireguard

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

const defaultMTU = 1420

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
// with a single mutex.
type Device struct {
	name    string
	address net.IPNet
	port    uint16
	mtu     int

	mu      sync.Mutex
	backend BackendDevice
	desired map[wgtypes.Key]PeerConfig
}

func newDevice(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	port uint16,
	mtu int,
	backend BackendDevice,
) *Device {
	if mtu <= 0 {
		mtu = defaultMTU
	}
	return &Device{
		name:    name,
		address: address,
		port:    port,
		mtu:     mtu,
		backend: backend,
		desired: make(map[wgtypes.Key]PeerConfig),
	}
}

func (d *Device) Name() string {
	return d.name
}

// SetPeers replaces the desired peer set and reconciles it against
// the live WireGuard state.
func (d *Device) SetPeers(peers ...PeerConfig) error {
	d.mu.Lock()
	desired := make(map[wgtypes.Key]PeerConfig, len(peers))
	for _, p := range peers {
		desired[p.PublicKey] = p
	}
	d.desired = desired
	d.mu.Unlock()

	return d.reconcile()
}

// Peers returns the live peer state from the WireGuard device.
func (d *Device) Peers() (
	[]PeerStatus,
	error,
) {
	d.mu.Lock()
	bd := d.backend
	d.mu.Unlock()

	return bd.Peers()
}

// UpdateEndpoint sets the endpoint for a single existing peer,
// bypassing the normal reconciliation flow. It updates the desired
// entry's endpoint and applies a targeted operation.
func (d *Device) UpdateEndpoint(
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
	if dp, ok := d.desired[key]; ok {
		dp.Endpoint = addr
		d.desired[key] = dp
	}
	bd := d.backend
	d.mu.Unlock()

	op := PeerOp{
		Config: PeerConfig{
			// AllowedIPs and Keepalive are zero-value; the backend will not set
			// them from this op since they use the zero-value. Only Endpoint is set.
			PublicKey: key,
			Endpoint:  addr,
		},
	}

	return bd.ApplyPeers([]PeerOp{op})
}

// Close brings the device down and removes it.
func (d *Device) Close() error {
	d.mu.Lock()
	bd := d.backend
	d.mu.Unlock()
	return bd.Close()
}

// reconcile observes the live device, plans changes, and applies
// only the operations needed to match the desired peer set.
func (d *Device) reconcile() error {
	d.mu.Lock()
	desired := d.desired
	bd := d.backend
	d.mu.Unlock()

	desiredSlice := make([]PeerConfig, 0, len(desired))
	for _, p := range desired {
		desiredSlice = append(desiredSlice, p)
	}

	observed, err := bd.Peers()
	if err != nil {
		return fmt.Errorf("wireguard: reconcile observe: %w", err)
	}

	ops := planPeerReconciliation(desiredSlice, observed)
	if len(ops) == 0 {
		return nil
	}

	return bd.ApplyPeers(ops)
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

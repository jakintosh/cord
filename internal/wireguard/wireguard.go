// Package wireguard provides a cross-platform WireGuard interface
// manager. It exposes a high-level WG interface for creating and
// managing WireGuard network devices, and handles backend selection
// (kernel or userspace) based on the platform and options.
package wireguard

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const maxInterfaceNameBytes = 15

// BackendType selects which WireGuard implementation drives a device.
type BackendType string

const (
	BackendAuto      BackendType = "auto"
	BackendKernel    BackendType = "kernel"
	BackendUserspace BackendType = "userspace"
)

// ParseBackendType converts a string to a BackendType, normalizing
// case and whitespace. An empty string returns BackendAuto. Returns
// an error for unrecognized values.
func ParseBackendType(s string) (BackendType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto", "":
		return BackendAuto, nil
	case "kernel":
		return BackendKernel, nil
	case "userspace":
		return BackendUserspace, nil
	default:
		return "", fmt.Errorf("unknown wireguard backend %q; valid: auto, kernel, userspace", s)
	}
}

// ValidBackendTypes returns the list of valid backend type strings.
func ValidBackendTypes() []string {
	return []string{string(BackendAuto), string(BackendKernel), string(BackendUserspace)}
}

// ValidateDeviceName checks that a device name fits within the kernel
// interface name length limit.
func ValidateDeviceName(name string) error {
	if len(name) > maxInterfaceNameBytes {
		return fmt.Errorf(
			"wireguard: device name %q exceeds %d byte limit",
			name,
			maxInterfaceNameBytes,
		)
	}
	return nil
}

// Options configures a WireGuard manager.
type Options struct {
	Backend BackendType
}

// WG manages WireGuard network devices — key generation, device
// creation, and device removal. A single WG instance can manage
// multiple devices, one per network.
type WG interface {
	// GenerateKey produces a new WireGuard private key.
	GenerateKey() (string, error)

	// PublicKey derives the public key from a private key.
	PublicKey(privateKey string) (string, error)

	// NewDevice creates a new WireGuard device (interface) with the
	// given name, private key, address, and listen port. The device
	// is not yet brought up. The name must not exceed
	// maxInterfaceNameBytes bytes (kernel limit).
	NewDevice(name string, privateKey string, address string, port uint16) (WGDevice, error)

	// RemoveDevice destroys a WireGuard device by name. The device
	// must be down before removal.
	RemoveDevice(name string) error
}

// WGDevice is a live WireGuard network device. ApplyPeers replaces
// the set of desired peers and reconciles them against the live
// WireGuard state.
type WGDevice interface {
	// ApplyPeers replaces the entire set of desired peers and
	// reconciles them against the live device. Unchanged peers are
	// not disturbed.
	ApplyPeers(peers []WGPeer) error

	// Up brings the device into the running state.
	Up() error

	// Down brings the device down without destroying it.
	Down() error

	// DeviceName returns the actual WireGuard interface name.
	DeviceName() string

	// WaitForHandshake blocks until the live device reports a
	// completed handshake with the peer identified by pubKey, or
	// until the timeout expires. The optional onStatus callback is
	// invoked with the live peer status each time it is observed.
	WaitForHandshake(pubKey string, timeout time.Duration, onStatus func(PeerStatus)) error

	// Status returns the live WireGuard device state, including
	// observed peer endpoints, handshake times, and byte counts.
	Status() ([]PeerStatus, error)
}

// EndpointPolicy controls how Cord manages a peer's endpoint.
type EndpointPolicy int

const (
	// EndpointDynamic leaves endpoint selection to WireGuard.
	EndpointDynamic EndpointPolicy = iota

	// EndpointBootstrap supplies an endpoint when adding a peer,
	// then allows WireGuard to roam.
	EndpointBootstrap

	// EndpointFixed continuously enforces the configured endpoint.
	EndpointFixed
)

// WGPeer is a single peer entry in a WireGuard device configuration.
// It carries only the fields WireGuard needs for configuration.
type WGPeer struct {
	PublicKey           string
	AllowedIPs          []string
	Endpoint            string // "host:port"; empty means dynamic endpoint
	PersistentKeepalive int    // seconds; 0 means no keepalive
	EndpointPolicy      EndpointPolicy
}

// PeerStatus is the observed runtime state of a WireGuard peer.
type PeerStatus struct {
	PublicKey     string
	Endpoint      string
	LastHandshake time.Time
	ReceiveBytes  int64
	TransmitBytes int64
}

// New returns a WG manager backed by the selected WireGuard
// implementation. On macOS only BackendUserspace is available; on
// Linux BackendKernel is the default (BackendAuto). The manager is
// safe for concurrent use.
func New(
	opts Options,
) (
	WG,
	error,
) {
	backend, err := newBackend(opts.Backend)
	if err != nil {
		return nil, fmt.Errorf("wireguard: new backend: %w", err)
	}
	return &manager{
		backend: backend,
		devices: make(map[string]*wgDevice),
	}, nil
}

// manager implements WG.
type manager struct {
	backend Backend
	devices map[string]*wgDevice
	mu      sync.Mutex
}

func (m *manager) GenerateKey() (
	string,
	error,
) {
	return GenerateKey()
}

func (m *manager) PublicKey(
	privateKey string,
) (
	string,
	error,
) {
	return PublicKey(privateKey)
}

func (m *manager) NewDevice(
	name string,
	privateKey string,
	address string,
	port uint16,
) (
	WGDevice,
	error,
) {
	if len(name) > maxInterfaceNameBytes {
		return nil, fmt.Errorf(
			"wireguard: interface name %q exceeds %d byte limit",
			name,
			maxInterfaceNameBytes,
		)
	}

	key, err := parseKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("wireguard: new device: %w", err)
	}

	_, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		return nil, fmt.Errorf("wireguard: new device: parse address %q: %w", address, err)
	}

	d := newDevice(name, key, *ipNet, port, 0, false, m.backend)

	m.mu.Lock()
	m.devices[name] = d
	m.mu.Unlock()

	return d, nil
}

func (m *manager) RemoveDevice(
	name string,
) error {
	m.mu.Lock()
	_, ok := m.devices[name]
	delete(m.devices, name)
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("wireguard: remove device: %q not found", name)
	}

	return m.backend.Delete(name)
}

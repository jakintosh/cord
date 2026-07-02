// Package wireguard provides a cross-platform WireGuard interface
// manager. It creates and manages WireGuard network devices, and
// handles backend selection (kernel or userspace) based on the
// platform and options.
package wireguard

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

// ErrDeviceNotUp is returned when an operation requiring a live
// WireGuard device is attempted before the device is brought up.
var ErrDeviceNotUp = errors.New("wireguard: device not up")

// EndpointPolicy controls how Cord manages a peer's endpoint.
type EndpointPolicy int

const (
	EndpointDynamic EndpointPolicy = iota
	EndpointBootstrap
	EndpointFixed
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
func ParseBackendType(
	s string,
) (
	BackendType,
	error,
) {
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

// ValidateDeviceName checks that a device name fits within the kernel
// interface name length limit.
func ValidateDeviceName(
	name string,
) error {
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

// Manager creates and tracks WireGuard devices. A single Manager
// instance can manage multiple devices, one per network, all sharing
// the same Backend.
type Manager struct {
	backend Backend
	devices map[string]*Device
	mu      sync.Mutex
}

// NewManager returns a Manager backed by the selected WireGuard
// implementation. On macOS only BackendUserspace is available; on
// Linux BackendKernel is the default (BackendAuto). The manager is
// safe for concurrent use.
func NewManager(
	opts Options,
) (
	*Manager,
	error,
) {
	backend, err := newBackend(opts.Backend)
	if err != nil {
		return nil, fmt.Errorf("wireguard: new backend: %w", err)
	}
	return NewManagerWithBackend(backend), nil
}

// NewManagerWithBackend returns a Manager that uses the given Backend
// directly. Most callers should use NewManager; this constructor exists
// for callers that need to supply their own Backend implementation.
func NewManagerWithBackend(
	backend Backend,
) *Manager {
	return &Manager{
		backend: backend,
		devices: make(map[string]*Device),
	}
}

// NewDevice creates a new WireGuard device (interface) with the
// given name, private key, interface address, and listen port. The
// address is the device's own address with its on-link prefix
// (e.g. 10.0.0.1/16), not a masked network — callers parsing a CIDR
// string should use netaddr.ParseInterface to preserve the host
// bits. The device is not yet brought up. The name must not exceed
// maxInterfaceNameBytes bytes (kernel limit).
func (m *Manager) NewDevice(
	name string,
	privateKey string,
	ifaceAddr net.IPNet,
	port uint16,
) (
	*Device,
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

	device := newDevice(name, key, ifaceAddr, port, 0, false, m.backend)

	m.mu.Lock()
	m.devices[name] = device
	m.mu.Unlock()

	return device, nil
}

// RemoveDevice destroys a WireGuard device by name. The device must
// be down before removal.
func (m *Manager) RemoveDevice(
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

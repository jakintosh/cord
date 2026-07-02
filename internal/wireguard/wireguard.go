// Package wireguard provides a cross-platform WireGuard interface
// manager. It creates and manages WireGuard network devices, and
// handles backend selection (kernel or userspace) based on the
// platform and options.
package wireguard

import (
	"fmt"
)

// EndpointPolicy controls how Cord manages a peer's endpoint.
type EndpointPolicy int

const (
	EndpointDynamic   EndpointPolicy = iota // cord manages (default)
	EndpointBootstrap                       // set only on initial add
	EndpointFixed                           // always reconciled
)

const maxInterfaceNameBytes = 15

// Options configures a WireGuard manager.
type Options struct {
	Backend BackendType
}

// Manager creates WireGuard devices. It is stateless beyond the
// shared Backend — it does not track devices.
type Manager struct {
	backend Backend
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
	}
}

// CreateDevice creates a new WireGuard device, brings it up, and
// returns the handle. Creating a device implies bringing it up —
// there is no separate "up" step. Call Close on the returned Device
// to tear it down.
func (m *Manager) CreateDevice(
	cfg DeviceConfig,
) (
	*Device,
	error,
) {
	if len(cfg.Name) > maxInterfaceNameBytes {
		return nil, fmt.Errorf("wireguard: interface name %q exceeds %d byte limit", cfg.Name, maxInterfaceNameBytes)
	}

	privKey, err := parseKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create device: %w", err)
	}

	bd, err := m.backend.CreateDevice(cfg)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create device: %w", err)
	}

	return newDevice(cfg.Name, privKey, cfg.Address, cfg.ListenPort, cfg.MTU, bd), nil
}

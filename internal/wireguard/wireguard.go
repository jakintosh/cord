// Package wireguard provides a cross-platform WireGuard interface
// manager. It creates and manages WireGuard network devices, and
// handles backend selection (kernel or userspace) based on the
// platform and options.
package wireguard

import (
	"fmt"
	"log/slog"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

const maxInterfaceNameBytes = 15

// Options configures a WireGuard manager.
type Options struct {
	Backend BackendType

	// Logger receives device lifecycle and reconciliation diagnostics.
	// Nil discards everything.
	Logger *slog.Logger
}

// Manager creates WireGuard devices. It is stateless beyond the
// shared Backend — it does not track devices.
type Manager struct {
	backend Backend
	log     *slog.Logger
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
	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}
	backend, err := newBackend(opts.Backend, log)
	if err != nil {
		return nil, fmt.Errorf("wireguard: new backend: %w", err)
	}
	return &Manager{
		backend: backend,
		log:     log,
	}, nil
}

// NewManagerWithBackend returns a Manager that uses the given Backend
// directly. Most callers should use NewManager; this constructor exists
// for callers that need to supply their own Backend implementation.
func NewManagerWithBackend(
	backend Backend,
) *Manager {
	return &Manager{
		backend: backend,
		log:     logging.Discard(),
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
	if err := ValidateDeviceName(cfg.Name); err != nil {
		return nil, err
	}

	_, err := parseKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create device: %w", err)
	}

	wgDevice, err := m.backend.CreateDevice(cfg)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create device: %w", err)
	}

	dev := newDevice(cfg.Name, wgDevice, m.log)
	dev.log.Debug("device created", "route", cfg.Route.String(), "port", cfg.ListenPort)
	return dev, nil
}

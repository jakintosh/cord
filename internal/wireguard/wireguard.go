// Package wireguard provides a cross-platform WireGuard interface
// manager. It creates and manages WireGuard network devices, and
// handles backend selection (kernel or userspace) based on the
// platform and options.
package wireguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

const maxInterfaceNameBytes = 15

// ActiveHandshakeThreshold is the maximum age of a WireGuard handshake that
// is considered current. Both clients and servers use it when deciding
// whether a learned endpoint is safe to retain or advertise. It covers
// WireGuard's normal rekey cycle while treating a key older than its rejection
// window as stale.
const ActiveHandshakeThreshold = 180 * time.Second

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

// NewDevice validates and retains configuration without creating an interface.
func (m *Manager) NewDevice(
	cfg DeviceConfig,
) (
	*Device,
	error,
) {
	if err := ValidateDeviceName(cfg.Name); err != nil {
		return nil, err
	}

	if _, err := parseKey(cfg.PrivateKey); err != nil {
		return nil, fmt.Errorf("wireguard: device config: %w", err)
	}

	return newDevice(cfg, m.backend, m.log), nil
}

// CreateDevice constructs and immediately reconciles a device for callers that
// need a running interface. If cleanup fails, a non-nil Device accompanies the
// error and must be closed again by the caller.
func (m *Manager) CreateDevice(
	cfg DeviceConfig,
) (
	*Device,
	error,
) {
	dev, err := m.NewDevice(cfg)
	if err != nil {
		return nil, err
	}

	if err := dev.Reconcile(context.Background()); err != nil {
		if stopErr := dev.Stop(); stopErr != nil {
			return dev, errors.Join(err, stopErr)
		}
		return nil, err
	}

	return dev, nil
}

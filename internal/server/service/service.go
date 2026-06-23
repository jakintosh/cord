package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// Options configures the domain core for the cord server. All fields are
// required for full operation but may be nil during early development.
type Options struct {
	// Store is the persistence adapter for server-side network state.
	Store Store

	// WG is the WireGuard abstraction for managing network devices.
	WG WG

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service.
	Logger *log.Logger
}

// Service is the domain core for the cord server. All domain operations
// are methods on Service, scoped by a network name parameter. It owns
// durable state through the Store and live WireGuard state through WG.
type Service struct {
	store Store
	wg    WG
	clock func() time.Time
	log   *log.Logger
	mu    sync.Mutex

	// running tracks networks that have been started (WG devices up).
	running map[string]*runningNetwork
}

// runningNetwork holds the live state for one started server network:
// the main and invite WireGuard devices plus a cancel function that
// stops the periodic reconciliation loop.
type runningNetwork struct {
	main   WGDevice
	invite WGDevice
	cancel context.CancelFunc
}

// New returns a ready-to-use Service. All Options fields are required
// for full operation but may be nil during early development — methods
// that depend on missing dependencies will return ErrNotImplemented.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	wg := opts.WG
	if wg == nil {
		wg = stubWG{}
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	return &Service{
		store:   opts.Store,
		wg:      wg,
		clock:   clock,
		log:     opts.Logger,
		running: make(map[string]*runningNetwork),
	}, nil
}

// Close shuts down all running networks, stops their reconciliation
// loops, and releases resources. It should be called during daemon
// shutdown.
func (s *Service) Close() error {
	return nil
}

// StartNetwork brings up both WireGuard devices (main and invite) for
// the named network and begins the periodic reconciliation loop. Blocks
// until ctx is cancelled. Idempotent: starting an already-running
// network is a no-op.
func (s *Service) StartNetwork(
	ctx context.Context,
	name string,
) error {
	return ErrNotImplemented
}

// StopNetwork brings down both WireGuard devices for the named network
// and stops the reconciliation loop. Idempotent.
func (s *Service) StopNetwork(
	name string,
) error {
	return ErrNotImplemented
}

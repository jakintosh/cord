package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// Options configures the domain core for the cord client daemon.
// Store and WG are required for full operation but may be nil during
// early development.
type Options struct {
	// Store is the persistence adapter for client-side network state.
	Store Store

	// WG is the WireGuard abstraction for managing network devices.
	WG WG

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service.
	Logger *log.Logger
}

// Service is the domain core for the cord client daemon. It manages
// client-side network memberships, their WireGuard interfaces, and
// background peer synchronization. All domain operations are methods on
// Service, scoped by a network name parameter.
//
// The service owns durable state through the Store and live WireGuard
// state through the WG abstraction.
type Service struct {
	store Store
	wg    WG
	clock func() time.Time
	log   *log.Logger
	mu    sync.Mutex

	// running tracks networks that are currently enabled (interface
	// up, sync loop active). The key is the network name.
	running map[string]*runningNetwork
}

// runningNetwork holds the live state for one enabled client network:
// the WireGuard device plus a cancel function that stops the
// background sync loop, and sync status tracking.
type runningNetwork struct {
	device   WGDevice
	cancel   context.CancelFunc
	lastSync time.Time
	lastErr  string
}

// New returns a ready-to-use Service. Store and WG may be nil during
// early development — methods that depend on them will return
// ErrNotImplemented. When WG is nil, a stub that generates random
// keys is used so that domain flows like InstallNetwork can proceed.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	wg := opts.WG
	if wg == nil {
		wg = stubWG{}
	}
	return &Service{
		store:   opts.Store,
		wg:      wg,
		clock:   opts.Clock,
		log:     opts.Logger,
		running: make(map[string]*runningNetwork),
	}, nil
}

// Start is called once at daemon startup. It loads every network with
// enabled=true from the store and enables each one (brings up
// interfaces, starts sync loops). Networks that fail to start are
// logged and left disabled.
//
// When the WG implementation is not available (stub), Start returns
// ErrNotImplemented since it cannot bring up real interfaces.
func (s *Service) Start(
	ctx context.Context,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	if _, ok := s.wg.(stubWG); ok {
		return ErrNotImplemented
	}

	names, err := s.store.ListNetworkNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		nw, err := s.store.GetNetwork(name)
		if err != nil {
			if s.log != nil {
				s.log.Printf("start: get network %q: %v", name, err)
			}
			continue
		}
		if !nw.Enabled {
			continue
		}

		if err := s.EnableNetwork(ctx, name); err != nil {
			if s.log != nil {
				s.log.Printf("start: enable network %q: %v", name, err)
			}
			_, _ = s.store.UpdateNetwork(name, UpdateNetworkRequest{
				Enabled: &[]bool{false}[0],
			})
		}
	}

	return nil
}

// Close shuts down all running networks (disables each one) and
// releases resources. It should be called during daemon shutdown.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name := range s.running {
		s.disableLocked(name)
	}
	return nil
}

package service

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// defaultEndpointTTL is how long an endpoint sighting is considered current.
const defaultEndpointTTL = 24 * time.Hour

// Options configures the domain core for the cord server.
type Options struct {
	// Store is the persistence adapter for server-side network state.
	Store Store

	// WireGuard is the WireGuard manager for managing network devices.
	WireGuard *wireguard.Manager

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service.
	Logger *log.Logger

	// APIFactory creates per-network HTTP handlers. Called when
	// starting a network. Nil means no API listeners are started.
	APIFactory func(network string) APIHandlers
}

// APIHandlers holds the HTTP handlers for a single network's main-facing
// and invite-facing APIs.
type APIHandlers struct {
	Main   http.Handler
	Invite http.Handler
}

// Service is the domain core for the cord server. All domain operations
// are methods on Service, scoped by a network name parameter. It owns
// durable state through the Store and live WireGuard state through WG.
type Service struct {
	store      Store
	wireguard  *wireguard.Manager
	clock      func() time.Time
	log        *log.Logger
	mu         sync.Mutex
	apiFactory func(network string) APIHandlers

	// running tracks networks that are up (WG devices + API servers).
	running map[string]*Network
}

// New returns a ready-to-use Service.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.WireGuard == nil {
		return nil, fmt.Errorf("server: wireguard manager required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	return &Service{
		store:      opts.Store,
		wireguard:  opts.WireGuard,
		clock:      clock,
		log:        opts.Logger,
		running:    make(map[string]*Network),
		apiFactory: opts.APIFactory,
	}, nil
}

// Start reads all persisted configs and starts every enabled network.
// Non-fatal per network: failures are logged and the rest continue.
func (s *Service) Start() error {
	names, err := s.store.ListNetworkNames()
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	var lastErr error
	for _, name := range names {
		nc, err := s.store.GetNetwork(name)
		if err != nil {
			s.logf("start networks: get %q: %v", name, err)
			lastErr = err
			continue
		}
		if !nc.Enabled {
			continue
		}
		if err := s.startNetwork(name); err != nil {
			s.logf("start networks: start %q: %v", name, err)
			lastErr = err
		}
	}
	return lastErr
}

// startNetwork constructs and starts a runtime Network from its
// persisted config. Must not be called while holding s.mu.
func (s *Service) startNetwork(
	name string,
) error {
	s.mu.Lock()
	if _, exists := s.running[name]; exists {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	nc, err := s.store.GetNetwork(name)
	if err != nil {
		return fmt.Errorf("start network: %w", mapStoreError(err))
	}

	n := &Network{
		config:   nc,
		store:    s.store,
		wg:       s.wireguard,
		api:      s.apiFactory,
		clock:    s.clock,
		logf:     s.logf,
		registry: func() map[string]*Network { return s.running },
	}

	if err := n.start(); err != nil {
		return fmt.Errorf("start network %q: %w", name, err)
	}

	s.mu.Lock()
	s.running[name] = n
	s.mu.Unlock()

	return nil
}

// Close shuts down all running networks, stops their reconciliation
// timers, and releases resources.
func (s *Service) Close() error {
	s.mu.Lock()
	networks := make([]*Network, 0, len(s.running))
	for _, n := range s.running {
		networks = append(networks, n)
	}
	s.running = make(map[string]*Network)
	s.mu.Unlock()

	var errs []error
	for _, n := range networks {
		if err := n.stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// logf writes a message to the service logger if configured.
func (s *Service) logf(format string, args ...any) {
	if s.log != nil {
		s.log.Printf(format, args...)
	}
}

// ResolveRegistrationIdentity looks up an unredeemed, unexpired
// registration by temporary IP within the invite network.
func (s *Service) ResolveRegistrationIdentity(
	networkName string,
	ip net.IP,
) (
	*Registration,
	error,
) {
	reg, err := s.store.GetRegistrationByIP(networkName, ip, s.clock())
	if err != nil {
		return nil, fmt.Errorf("resolve registration identity: %w", mapStoreError(err))
	}
	return reg, nil
}

// ResolvePeerIdentity looks up a confirmed, enabled peer by IP address
// within the network.
func (s *Service) ResolvePeerIdentity(
	network string,
	ip net.IP,
) (
	*Peer,
	error,
) {
	p, err := s.store.GetPeerByIP(network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve peer identity: %w", mapStoreError(err))
	}
	return p, nil
}

// ResolveProvisionalIdentity looks up an unconfirmed, enabled peer by
// IP address within the network.
func (s *Service) ResolveProvisionalIdentity(
	network string,
	ip net.IP,
) (
	*Peer,
	error,
) {
	p, err := s.store.GetProvisionalPeerByIP(network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve provisional identity: %w", mapStoreError(err))
	}
	return p, nil
}

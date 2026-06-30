package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Options configures the domain core for the cord server. All fields are
// required for full operation but may be nil during early development.
type Options struct {
	// Store is the persistence adapter for server-side network state.
	Store Store

	// WG is the WireGuard manager for managing network devices.
	WG wireguard.WG

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service.
	Logger *log.Logger

	// ReconcileInterval controls how often the reconciliation loop runs
	// for each started network. Defaults to 10s when zero.
	ReconcileInterval time.Duration

	// APIFactory creates per-network HTTP handlers. Called by
	// StartNetwork. Nil means no API listeners are started.
	APIFactory func(network string) APIHandlers
}

// APIHandlers holds the HTTP handlers for a single network's main-facing
// and invite-facing APIs. Created by APIFactory and used internally by
// StartNetwork to start HTTP listeners.
type APIHandlers struct {
	Main   http.Handler
	Invite http.Handler
}

// Service is the domain core for the cord server. All domain operations
// are methods on Service, scoped by a network name parameter. It owns
// durable state through the Store and live WireGuard state through WG.
type Service struct {
	store             Store
	wg                wireguard.WG
	clock             func() time.Time
	log               *log.Logger
	mu                sync.Mutex
	reconcileInterval time.Duration
	apiFactory        func(network string) APIHandlers

	// running tracks networks that have been started (WG devices up).
	running map[string]*NetworkDevices
}

// NetworkDevices holds the live WireGuard state for a started server
// network: both devices, their interface names, and a cancel function
// that stops the reconciliation loop.
type NetworkDevices struct {
	MainName     string
	MainDevice   wireguard.WGDevice
	MainServer   *http.Server
	InviteName   string
	InviteDevice wireguard.WGDevice
	InviteServer *http.Server
	Cancel       context.CancelFunc
}

// New returns a ready-to-use Service. All Options fields are required
// for full operation but may be nil during early development — methods
// that depend on missing dependencies will return appropriate errors.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.WG == nil {
		return nil, fmt.Errorf("server: wireguard manager required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	reconcileInterval := opts.ReconcileInterval
	if reconcileInterval == 0 {
		reconcileInterval = 10 * time.Second
	}

	return &Service{
		store:             opts.Store,
		wg:                opts.WG,
		clock:             clock,
		log:               opts.Logger,
		running:           make(map[string]*NetworkDevices),
		reconcileInterval: reconcileInterval,
		apiFactory:        opts.APIFactory,
	}, nil
}

// Close shuts down all running networks, stops their reconciliation
// loops, and releases resources. It should be called during daemon
// shutdown.
func (s *Service) Close() error {
	s.mu.Lock()
	names := make([]string, 0, len(s.running))
	for name := range s.running {
		names = append(names, name)
	}
	s.mu.Unlock()

	var errs []error
	for _, name := range names {
		if err := s.StopNetwork(name); err != nil {
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

// ResolveInviteIdentity looks up an unredeemed, unexpired invite by
// temporary IP within the invite network. Used by the identity middleware
// to authenticate incoming invite-redemption requests.
func (s *Service) ResolveInviteIdentity(
	networkName string,
	ip net.IP,
) (
	*Invite,
	error,
) {
	inv, err := s.store.GetInviteByIP(networkName, ip, s.clock())
	if err != nil {
		return nil, fmt.Errorf("resolve invite identity: %w", mapStoreError(err))
	}
	return inv, nil
}

// ResolvePeerIdentity looks up a confirmed, enabled peer by IP address
// within the network. Used by the identity middleware to authenticate
// ordinary peer API calls (/peers, /endpoints) by WireGuard source IP.
func (s *Service) ResolvePeerIdentity(
	network string,
	ip net.IP,
) (*Peer, error) {
	p, err := s.store.GetPeerByIP(network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve peer identity: %w", mapStoreError(err))
	}
	return p, nil
}

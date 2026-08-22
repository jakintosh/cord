package service

import (
	"fmt"
	"log/slog"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

// defaultEndpointTTL is how long an endpoint sighting is considered current.
const defaultEndpointTTL = 24 * time.Hour

// Options configures the domain core for the cord server.
type Options struct {
	// Store is the persistence adapter for server-side network state.
	Store Store

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service. Nil
	// discards everything.
	Logger *slog.Logger

	// Wake receives the name of a network whose derived state changed,
	// so the runtime can converge it promptly. Nil means nothing is
	// listening; sends are dropped when the channel is full.
	Wake chan<- string
}

// Service is the domain core for the cord server. All domain operations
// are methods on Service, scoped by a network name parameter. It owns
// durable state through the Store and computes the desired WireGuard
// state, but never touches devices, listeners, goroutines, or timers.
type Service struct {
	store Store
	clock func() time.Time
	log   *slog.Logger
	wake  chan<- string
}

// New returns a ready-to-use Service.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.Store == nil {
		return nil, fmt.Errorf("server: store required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}

	return &Service{
		store: opts.Store,
		clock: clock,
		log:   log,
		wake:  opts.Wake,
	}, nil
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

// requestReconcile tells the runtime that the named network's desired
// state changed. The send never blocks: a full channel already holds a
// wake the runtime has not consumed yet, and its periodic pass catches
// up.
func (s *Service) requestReconcile(
	network string,
) {
	if s.wake == nil {
		return
	}
	select {
	case s.wake <- network:
	default:
	}
}

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
	Main         wireguard.WGDevice
	MainServer   *http.Server
	InviteName   string
	Invite       wireguard.WGDevice
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

// firstAssignableIP returns the first usable IP in a network
// (network address + 1, which is typically the server's own address).
func firstAssignableIP(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)
	ip[len(ip)-1]++
	return normalizeIP(ip)
}

// cidrRange returns the network address and broadcast address.
func cidrRange(n *net.IPNet) (net.IP, net.IP) {
	first := n.IP.Mask(n.Mask)
	last := make(net.IP, len(first))
	copy(last, first)
	for i := range last {
		last[i] |= ^n.Mask[i]
	}
	return normalizeIP(first), normalizeIP(last)
}

// incrementIP adds 1 to an IP address.
func incrementIP(ip net.IP) net.IP {
	ip = normalizeIP(ip)
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] > 0 {
			break
		}
	}
	return next
}

// normalizeIP converts an IPv4-in-IPv6 to a 4-byte representation.
func normalizeIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// terminalPrefix returns /32 for v4 or /128 for v6 — host routes for WireGuard peers.
func terminalPrefix(ip net.IP) int {
	if ip.To4() != nil {
		return 32
	}
	return 128
}

// interfaceAddress returns a CIDR notation string for the first
// assignable IP in a network, preserving the network prefix length.
func interfaceAddress(n *net.IPNet) string {
	ip := firstAssignableIP(n)
	prefix, _ := n.Mask.Size()
	return fmt.Sprintf("%s/%d", ip.String(), prefix)
}

// hostRoute returns a terminal route for a peer IP.
func hostRoute(ip net.IP) string {
	ip = normalizeIP(ip)
	return fmt.Sprintf("%s/%d", ip.String(), terminalPrefix(ip))
}

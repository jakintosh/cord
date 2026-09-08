// Package dns serves the authoritative Cord namespace for one server
// network: UDP and TCP listeners on the network's Cord server address
// that answer forward lookups within the network's <network>.internal
// zone. The package owns DNS protocol handling and listener lifecycle;
// naming, visibility, and addressability policy stay in the server
// service, which it receives as a concrete dependency.
package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"github.com/miekg/dns"
)

const shutdownTimeout = 5 * time.Second

// Options configures one authoritative DNS server.
type Options struct {
	// Network is the Cord network whose peers are served. Service
	// operations are scoped by it.
	Network string

	// Zone is the served namespace, e.g. "example.internal". It is
	// normalized to a lowercase fully qualified name.
	Zone string

	// Address is the "ip:port" bind address. UDP and TCP share the
	// same port; an OS-assigned ephemeral port is read back from the
	// bound server.
	Address string

	// Service is the server domain core used for source identity and
	// host resolution.
	Service *service.Service

	// Logger receives listener and request diagnostics. Nil discards
	// everything.
	Logger *slog.Logger
}

// Server is one authoritative DNS server for a Cord network, serving
// UDP and TCP on a single port. Constructors are synchronous and
// launch nothing: Reconcile binds both sockets before reporting success,
// Stop releases them and permits restart, forcing socket cleanup when the
// grace period expires. Forced cleanup does not wait for handlers.
type Server struct {
	network string
	zone    string
	address string
	service *service.Service
	log     *slog.Logger

	// mu serializes lifecycle operations and protects status snapshots.
	// Non-nil handles retain ownership until Stop.
	mu      sync.Mutex
	udp     *dns.Server
	tcp     *dns.Server
	lastErr error
}

// New validates the options and returns a server that is not yet
// serving. No sockets are bound and no goroutines are launched.
func New(
	opts Options,
) (
	*Server,
	error,
) {
	if opts.Service == nil {
		return nil, fmt.Errorf("dns: service required")
	}

	if opts.Network == "" {
		return nil, fmt.Errorf("dns: network required")
	}

	if opts.Zone == "" {
		return nil, fmt.Errorf("dns: zone required")
	}

	host, port, err := net.SplitHostPort(opts.Address)
	if err != nil {
		return nil, fmt.Errorf("dns: invalid address %q: %v", opts.Address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("dns: invalid address %q: host is not an IP address", opts.Address)
	}

	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, fmt.Errorf("dns: invalid address %q: %v", opts.Address, err)
	}

	zone := strings.ToLower(dns.Fqdn(opts.Zone))
	if _, ok := dns.IsDomainName(zone); !ok {
		return nil, fmt.Errorf("dns: invalid zone %q", opts.Zone)
	}

	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}

	return &Server{
		network: opts.Network,
		zone:    zone,
		address: net.JoinHostPort(ip.String(), port),
		service: opts.Service,
		log:     log,
	}, nil
}

// Status reports current listener availability without changing state or
// repairing resources. Closed sockets are detected even before reconciliation.
func (s *Server) Status() resource.Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.statusLocked()
}

// Reconcile ensures the configured server is running. The mutex serializes
// the entire check, cleanup, and start sequence with other resource calls.
func (s *Server) Reconcile(
	ctx context.Context,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.statusLocked().State {
	case resource.Running:
		return nil
	case resource.Stopped:
		break
	case resource.Failed:
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		err := s.stopLocked(stopCtx)
		cancel()
		if err != nil {
			return err
		}
	}

	if err := s.startLocked(ctx); err != nil {
		return err
	}

	return nil
}

// Stop shuts down UDP and TCP concurrently, giving each server until ctx
// expires to finish gracefully. A failed shutdown is followed by Close.
// Stop releases the underlying servers and returns shutdown and close errors. Forced
// socket cleanup does not wait for handlers to return. Repeated calls are no-ops.
func (s *Server) Stop(
	ctx context.Context,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stopLocked(ctx)
}

// Address returns the address both listeners are bound to: the
// configured address before startup, and the actual bound address —
// including an OS-assigned ephemeral port — afterwards.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.udp != nil && s.udp.PacketConn != nil {
		return s.udp.PacketConn.LocalAddr().String()
	}

	return s.address
}

// statusLocked requires mu and reads availability without changing state.
func (s *Server) statusLocked() resource.Status {
	if s.lastErr != nil {
		return resource.Status{
			State: resource.Failed,
			Error: s.lastErr,
		}
	}

	if s.udp == nil && s.tcp == nil {
		return resource.Status{
			State: resource.Stopped,
		}
	}

	if err := s.probeSocketsLocked(); err != nil {
		return resource.Status{
			State: resource.Failed,
			Error: err,
		}
	}

	return resource.Status{
		State: resource.Running,
	}
}

// startLocked requires mu and binds UDP and TCP on the same port.
func (s *Server) startLocked(
	ctx context.Context,
) (
	err error,
) {
	if s.udp != nil || s.tcp != nil {
		return fmt.Errorf("dns: server already started")
	}

	defer func() { s.lastErr = err }()

	if err := ctx.Err(); err != nil {
		return err
	}

	handler := dns.HandlerFunc(s.serveDNS)
	udp, address, err := s.startUDP(handler)
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		_ = udp.Close()
		return err
	}

	tcp, err := s.startTCP(handler, address)
	if err != nil {
		_ = udp.Close()
		return err
	}

	s.udp, s.tcp = udp, tcp
	s.log.Info("dns recovered", "address", address)
	return nil
}

// stopLocked requires mu and releases the current serving attempt.
func (s *Server) stopLocked(
	ctx context.Context,
) error {
	if s.udp == nil && s.tcp == nil {
		return nil
	}

	// Preserve an existing failure through cleanup; only a successful start
	// establishes recovery. A normal stop of healthy listeners remains stopped.
	priorErr := s.lastErr
	if priorErr == nil {
		priorErr = s.probeSocketsLocked()
	}

	var wg sync.WaitGroup
	var udpErr, tcpErr error
	if s.udp != nil {
		wg.Go(func() {
			if err := s.udp.ShutdownContext(ctx); err != nil {
				udpErr = fmt.Errorf("dns: udp shutdown: %w", err)
				if err := s.udp.Close(); err != nil {
					udpErr = errors.Join(udpErr, fmt.Errorf("dns: udp force close: %w", err))
				}
			}
		})
	}
	if s.tcp != nil {
		wg.Go(func() {
			if err := s.tcp.ShutdownContext(ctx); err != nil {
				tcpErr = fmt.Errorf("dns: tcp shutdown: %w", err)
				if err := s.tcp.Close(); err != nil {
					tcpErr = errors.Join(tcpErr, fmt.Errorf("dns: tcp force close: %w", err))
				}
			}
		})
	}
	wg.Wait()

	s.udp, s.tcp = nil, nil
	stopErr := errors.Join(udpErr, tcpErr)
	s.lastErr = errors.Join(priorErr, stopErr)
	return stopErr
}

// socketsLocked requires mu and returns the raw UDP and TCP sockets behind the serving
// loops, or nils when they are not available.
func (s *Server) socketsLocked() (
	*net.UDPConn,
	*net.TCPListener,
) {
	var udpConn *net.UDPConn
	var tcpLn *net.TCPListener

	if s.udp != nil {
		udpConn, _ = s.udp.PacketConn.(*net.UDPConn)
	}
	if s.tcp != nil {
		tcpLn, _ = s.tcp.Listener.(*net.TCPListener)
	}

	return udpConn, tcpLn
}

// probeSocketsLocked requires mu and reports the first failure among the serving loops'
// sockets. A closed connection fails its raw-control probe
// immediately and without side effects on live serving.
func (s *Server) probeSocketsLocked() error {
	udpConn, tcpLn := s.socketsLocked()

	if udpConn != nil {
		if err := probeRaw(udpConn); err != nil {
			return fmt.Errorf("udp socket: %w", err)
		}
	}

	if tcpLn != nil {
		if err := probeRaw(tcpLn); err != nil {
			return fmt.Errorf("tcp socket: %w", err)
		}
	}

	return nil
}

// startUDP binds the UDP socket and launches its serving loop,
// returning the server once the socket is live, together with the
// address it actually bound. UDP must bind through the library so it
// applies its socket options; the bound address — not the configured
// one, which may carry an OS-assigned ephemeral port — is what TCP
// must share. Once live the loop runs until Shutdown, and Shutdown
// itself joins it, so no other bookkeeping is needed for the running
// lifetime.
func (s *Server) startUDP(
	handler dns.Handler,
) (
	*dns.Server,
	string,
	error,
) {
	srv := &dns.Server{
		Addr:    s.address,
		Net:     "udp",
		Handler: handler,
	}

	live := make(chan struct{})
	srv.NotifyStartedFunc = func() {
		close(live)
	}

	// ListenAndServe returns only with a bind error while starting;
	// the result channel is buffered so the final nil at Shutdown
	// never blocks.
	result := make(chan error, 1)
	go func() {
		result <- srv.ListenAndServe()
	}()

	select {
	case <-live:
		address := srv.PacketConn.LocalAddr()
		return srv, address.String(), nil
	case err := <-result:
		return nil, "", fmt.Errorf("dns: bind udp on %s: %w", s.address, err)
	}
}

// startTCP binds TCP on address, the address the UDP socket
// actually bound, so both protocols answer on one port, and launches
// its serving loop. Binding here, not through the library, makes a
// port conflict a plain synchronous error. The listener is already
// bound when the loop starts, so ActivateAndServe cannot fail before
// notifying; the liveness wait only ensures the loop's internal
// state is initialized before start reports success and Stop could
// race it.
func (s *Server) startTCP(
	handler dns.Handler,
	address string,
) (
	*dns.Server,
	error,
) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dns: bind tcp on %s: %w", address, err)
	}

	srv := &dns.Server{
		Listener: ln,
		Net:      "tcp",
		Handler:  handler,
	}

	live := make(chan struct{})
	srv.NotifyStartedFunc = func() {
		close(live)
	}

	go func() {
		_ = srv.ActivateAndServe()
	}()

	<-live

	return srv, nil
}

// probeRaw runs a no-op control operation on the socket's raw
// connection; a closed socket fails it with a use-of-closed error.
func probeRaw(
	conn syscall.Conn,
) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	return raw.Control(func(uintptr) {})
}

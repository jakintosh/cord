// Package dns serves the authoritative Cord namespace for one server
// network: UDP and TCP listeners on the network's Cord server address
// that answer forward lookups within the network's <network>.internal
// zone. The package owns DNS protocol handling and listener lifecycle;
// naming, visibility, and addressability policy stay in the server
// service, which it receives as a concrete dependency.
package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

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

// Status is a snapshot of one DNS server's listener state, following
// the daemon's activity-status style.
type Status struct {
	Serving       bool
	LastAttemptAt time.Time
	LastSuccessAt time.Time
	Error         string
}

// Server is one authoritative DNS server for a Cord network, serving
// UDP and TCP on a single port. Constructors are synchronous and
// launch nothing: Start binds both sockets before reporting success,
// Stop releases them, and the server is restartable afterwards.
type Server struct {
	network string
	zone    string
	address string
	service *service.Service
	log     *slog.Logger

	// mu guards the serving state. The sockets behind the library's
	// servers are the single source of health: a closed socket is
	// observable through its raw connection without disturbing live
	// serving.
	mu            sync.Mutex
	serving       bool
	udp           *dns.Server
	tcp           *dns.Server
	lastAttemptAt time.Time
	lastSuccessAt time.Time
	lastErr       error
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

	zone := strings.ToLower(dnsutil.Fqdn(opts.Zone))
	if !dnsutil.IsName(zone) {
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

// Start serves UDP, then TCP on the port UDP bound, and reports
// success only once both sockets are bound and their loops are
// serving. TCP binds whatever port UDP landed on, which may be an
// OS-assigned ephemeral port. A failure at either step shuts down
// whatever already started, so a returned error means a fully
// rolled-back start. ctx is honored between the two binds; a bind
// syscall cannot block, so there is nothing to wait on mid-bind. The
// server refuses a second Start while serving and is restartable
// after Stop.
func (s *Server) Start(
	ctx context.Context,
) error {
	if err := s.beginStart(); err != nil {
		return err
	}

	handler := dns.HandlerFunc(s.serveDNS)

	udp, address, err := s.startUDP(handler)
	if err != nil {
		s.endStart(nil, nil, err)
		return err
	}

	if err := ctx.Err(); err != nil {
		udp.Shutdown(context.Background())
		s.endStart(nil, nil, err)
		return err
	}

	tcp, err := s.startTCP(handler, address)
	if err != nil {
		udp.Shutdown(context.Background())
		s.endStart(nil, nil, err)
		return err
	}

	s.endStart(udp, tcp, nil)

	return nil
}

// Stop shuts down both servers and releases the sockets, bounded by
// ctx. Each library Shutdown joins its own serving loop, so no
// separate wait is needed. If ctx expires first, the sockets are
// force-closed and ctx's error is returned; the loops finish
// shutting down in the background. Stop is safe to call twice and
// before Start.
func (s *Server) Stop(
	ctx context.Context,
) error {
	s.mu.Lock()
	if !s.serving {
		s.mu.Unlock()
		return nil
	}
	s.serving = false
	udp, tcp := s.udp, s.tcp
	udpConn, tcpLn := s.sockets()
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Shutdown blocks until its serving loop has exited, so
		// both loops are joined once it returns.
		if udp != nil {
			udp.Shutdown(ctx)
		}
		if tcp != nil {
			tcp.Shutdown(ctx)
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		if udpConn != nil {
			_ = udpConn.Close()
		}
		if tcpLn != nil {
			_ = tcpLn.Close()
		}
		return fmt.Errorf("dns: stop %s: %w", s.address, ctx.Err())
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.udp, s.tcp = nil, nil

	return nil
}

// Status reports whether the server is serving, the most recent start
// attempt and success, and the most recent error. Health is read from
// the sockets themselves, so an unexpected listener exit flips Serving
// without any background watcher.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := Status{
		LastAttemptAt: s.lastAttemptAt,
		LastSuccessAt: s.lastSuccessAt,
	}
	if s.lastErr != nil {
		status.Error = s.lastErr.Error()
	}

	if s.serving && s.udp != nil && s.tcp != nil {
		if err := s.probeSockets(); err != nil {
			status.Error = err.Error()
		} else {
			status.Serving = true
		}
	}

	return status
}

// Address returns the address both listeners are bound to: the
// configured address before Start, and the actual bound address —
// including an OS-assigned ephemeral port — afterwards.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.udp != nil && s.udp.PacketConn != nil {
		return s.udp.PacketConn.LocalAddr().String()
	}

	return s.address
}

// beginStart refuses a second Start while one is serving and records
// the attempt time.
func (s *Server) beginStart() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.serving {
		return fmt.Errorf("dns: server already started")
	}

	s.serving = true
	s.lastAttemptAt = time.Now()

	return nil
}

// endStart publishes the sockets of a successful start, or unwinds
// to not-serving so a failed attempt can be retried, and records the
// outcome.
func (s *Server) endStart(
	udp *dns.Server,
	tcp *dns.Server,
	err error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		s.serving = false
		s.lastErr = err
		return
	}

	s.udp = udp
	s.tcp = tcp
	s.lastSuccessAt = time.Now()
	s.lastErr = nil
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
	srv.NotifyStartedFunc = func(context.Context) {
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
// bound when the loop starts, so ListenAndServe cannot fail before
// notifying; the liveness wait only ensures the loop's internal
// state is initialized before Start reports success and Stop could
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
	srv.NotifyStartedFunc = func(context.Context) {
		close(live)
	}

	go func() {
		_ = srv.ListenAndServe()
	}()

	<-live

	return srv, nil
}

// sockets returns the raw UDP and TCP sockets behind the serving
// loops, or nils when they are not available.
func (s *Server) sockets() (
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

// probeSockets reports the first failure among the serving loops'
// sockets. A closed connection fails its raw-control probe
// immediately and without side effects on live serving.
func (s *Server) probeSockets() error {
	udpConn, tcpLn := s.sockets()

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

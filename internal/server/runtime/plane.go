package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/dns"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const defaultShutdownTimeout = 5 * time.Second

// dnsConfig enables authoritative DNS when constructing a plane.
type dnsConfig struct {
	network string
	service *service.Service
	port    uint16
}

// httpServer owns configured HTTP serving across starts, failures, and stops.
type httpServer struct {
	address string
	handler http.Handler
	log     *slog.Logger
	timeout time.Duration

	mu      sync.Mutex
	run     *httpRun
	lastErr error
}

// httpRun holds one serving attempt. Its result cannot affect a later attempt.
type httpRun struct {
	server *http.Server
	cancel context.CancelFunc
	done   chan struct{}
	err    error // Written before done closes; read only after it closes.
}

// Status reads current availability without repairing or consuming the result.
func (s *httpServer) Status() resource.Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.statusLocked()
}

// Reconcile ensures the configured server is running. The mutex serializes
// the entire check, cleanup, and start sequence with other resource calls.
func (s *httpServer) Reconcile(
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
		stopCtx, cancel := context.WithTimeout(context.Background(), s.timeout)
		err := s.stopLocked(stopCtx)
		cancel()
		if err != nil {
			return err
		}
	}

	if err := s.startLocked(ctx); err != nil {
		return err
	}

	s.log.Info("http recovered", "address", s.address)

	return nil
}

// Stop releases the underlying server, joining its serving loop. A failure
// already observed before cleanup remains available until a successful start.
func (s *httpServer) Stop(
	ctx context.Context,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stopLocked(ctx)
}

// statusLocked requires mu and reads availability without changing state.
func (s *httpServer) statusLocked() resource.Status {
	if s.run != nil {
		select {
		case <-s.run.done:
			return resource.Status{
				State: resource.Failed,
				Error: s.run.err,
			}
		default:
			return resource.Status{
				State: resource.Running,
			}
		}
	}

	if s.lastErr != nil {
		return resource.Status{
			State: resource.Failed,
			Error: s.lastErr,
		}
	}

	return resource.Status{
		State: resource.Stopped,
	}
}

// startLocked requires mu and starts a fresh underlying server.
func (s *httpServer) startLocked(
	ctx context.Context,
) (
	err error,
) {
	if s.run != nil {
		return fmt.Errorf("http: server already started")
	}
	defer func() { s.lastErr = err }()

	if err := ctx.Err(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("http listen on %s: %w", s.address, err)
	}

	requestCtx, cancel := context.WithCancel(ctx)
	run := &httpRun{
		server: &http.Server{
			Addr:    listener.Addr().String(),
			Handler: s.handler,
			BaseContext: func(net.Listener) context.Context {
				return requestCtx
			},
		},
		cancel: cancel,
		done:   make(chan struct{}),
	}
	s.run = run

	go func() {
		run.err = run.server.Serve(listener)
		if !errors.Is(run.err, http.ErrServerClosed) {
			s.log.Error(
				"http server exited",
				"addr",
				run.server.Addr,
				"err",
				run.err,
			)
		}
		close(run.done)
	}()

	return nil
}

// stopLocked requires mu and releases the current serving attempt.
func (s *httpServer) stopLocked(
	ctx context.Context,
) error {
	if s.run == nil {
		return nil
	}
	run := s.run

	select {
	case <-run.done:
		s.lastErr = run.err
	default:
		// still running, no run.err
	}

	run.cancel()

	err := run.server.Shutdown(ctx)
	if err != nil {
		err = fmt.Errorf("http shutdown: %w", err)
		if closeErr := run.server.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("http force close: %w", closeErr))
		}
	}

	<-run.done

	s.run = nil
	s.lastErr = errors.Join(s.lastErr, err)

	return err
}

// PlaneStatus is a snapshot of the plane's configured child resources.
type PlaneStatus struct {
	WireGuard resource.Status
	DNS       resource.Status
	HTTP      resource.Status
}

// Plane owns persistent resource wrappers and decides when to recover them.
type Plane struct {
	config service.Plane
	device *wireguard.Device
	http   *httpServer
	dns    *dns.Server

	mu      sync.Mutex
	timeout time.Duration
}

// newPlane constructs configured wrappers without starting any resources.
func newPlane(
	config service.Plane,
	dnsConfig *dnsConfig,
	privateKey string,
	handler http.Handler,
	wg *wireguard.Manager,
	timeout time.Duration,
	log *slog.Logger,
) (
	*Plane,
	error,
) {
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	route, err := netaddr.InterfaceRouteFromCidr(config.Cidr)
	if err != nil {
		return nil, fmt.Errorf("plane address: %w", err)
	}

	_, networkCIDR, err := net.ParseCIDR(config.Cidr)
	if err != nil {
		return nil, err
	}

	device, err := wg.NewDevice(wireguard.DeviceConfig{
		Name:        config.Name,
		PrivateKey:  privateKey,
		Route:       route,
		NetworkCIDR: *networkCIDR,
		ListenPort:  config.WireguardPort,
	})
	if err != nil {
		return nil, fmt.Errorf("create device %q: %w", config.Name, err)
	}

	var httpSrv *httpServer = nil
	if handler != nil {
		httpSrv = &httpServer{
			address: netaddr.Endpoint(route.IP, config.ApiPort),
			handler: handler,
			log:     log,
			timeout: timeout,
		}
	}

	var dnsSrv *dns.Server = nil
	if dnsConfig != nil {
		if dnsConfig.port == 0 {
			dnsConfig.port = 53
		}

		dnsSrv, err = dns.New(dns.Options{
			Network: dnsConfig.network,
			Zone:    dnsConfig.network + ".internal",
			Address: netaddr.Endpoint(route.IP, dnsConfig.port),
			Service: dnsConfig.service,
			Logger:  log,
		})
		if err != nil {
			return nil, fmt.Errorf("dns: %w", err)
		}
	}

	return &Plane{
		config:  config,
		device:  device,
		http:    httpSrv,
		dns:     dnsSrv,
		timeout: timeout,
	}, nil
}

// Status collects child status without changing resources. The plane mutex
// serializes the snapshot with its reconciliation and shutdown sequences.
func (p *Plane) Status() PlaneStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	status := PlaneStatus{
		WireGuard: p.device.Status(),
		DNS: resource.Status{
			State: resource.Disabled,
		},
		HTTP: resource.Status{
			State: resource.Disabled,
		},
	}
	if p.dns != nil {
		status.DNS = p.dns.Status()
	}
	if p.http != nil {
		status.HTTP = p.http.Status()
	}
	return status
}

// Reconcile attempts every resource, starting with the interface, and joins failures.
func (p *Plane) Reconcile(
	ctx context.Context,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	if err := p.device.Reconcile(ctx); err != nil {
		errs = append(errs, fmt.Errorf("device %q: %w", p.config.Name, err))
	}

	if p.dns != nil {
		errs = append(errs, p.dns.Reconcile(ctx))
	}

	if p.http != nil {
		errs = append(errs, p.http.Reconcile(ctx))
	}

	return errors.Join(errs...)
}

// Stop releases servers before the device, retaining their configured wrappers.
func (p *Plane) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	if p.dns != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		errs = append(errs, p.dns.Stop(ctx))
		cancel()
	}

	if p.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		errs = append(errs, p.http.Stop(ctx))
		cancel()
	}

	if p.device != nil {
		if err := p.device.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("device close %q: %w", p.config.Name, err))
		}
	}

	return errors.Join(errs...)
}

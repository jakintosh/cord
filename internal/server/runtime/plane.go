package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/server/dns"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// shutdownTimeout bounds each plane server's graceful shutdown.
const shutdownTimeout = 5 * time.Second

// dnsConfig enables authoritative DNS for a plane.
type dnsConfig struct {
	network string
	service *service.Service
	port    uint16
}

// Plane owns a WireGuard device, optional HTTP API, and optional DNS server.
type Plane struct {
	config     service.Plane
	dnsConfig  *dnsConfig
	privateKey string

	device *wireguard.Device
	server *http.Server
	dns    *dns.Server

	ctx           context.Context
	clock         func() time.Time
	dnsStatus     ActivityStatus
	log           *slog.Logger
	onServeResult func(error)
}

// start creates the WireGuard device, then synchronously starts the HTTP
// API listener and optional DNS server, so bind failures are synchronous.
// The network calls stop to roll back a failed start. HTTP requests inherit ctx.
func (p *Plane) start(
	ctx context.Context,
	wg *wireguard.Manager,
	handler http.Handler,
) error {
	p.ctx = ctx

	// determine interface address
	ifaceRoute, err := netaddr.InterfaceRouteFromCidr(p.config.Cidr)
	if err != nil {
		return fmt.Errorf("parse cidr: %v", err)
	}

	_, networkCIDR, err := net.ParseCIDR(p.config.Cidr)
	if err != nil {
		return fmt.Errorf("parse cidr: %v", err)
	}

	// create wg device
	p.device, err = wg.CreateDevice(wireguard.DeviceConfig{
		Name:        p.config.Name,
		PrivateKey:  p.privateKey,
		Route:       ifaceRoute,
		NetworkCIDR: *networkCIDR,
		ListenPort:  p.config.WireguardPort,
	})
	if err != nil {
		return fmt.Errorf("create device %q: %w", p.config.Name, err)
	}

	// start API server if handler is provided
	if handler != nil {
		apiEndpoint := netaddr.Endpoint(ifaceRoute.IP, p.config.ApiPort)
		ln, err := net.Listen("tcp", apiEndpoint)
		if err != nil {
			// A failed start leaves the plane holding nothing: the
			// device is closed and forgotten, so the plane reads as
			// never started and a later stop or retry is a no-op.
			_ = p.device.Close()
			p.device = nil
			return fmt.Errorf("listen %q on %s: %w", p.config.Name, apiEndpoint, err)
		}

		p.server = &http.Server{
			Addr:        apiEndpoint,
			Handler:     handler,
			BaseContext: func(net.Listener) context.Context { return ctx },
		}
		p.log.Debug("api listening", "addr", apiEndpoint)
		p.serve(ln, apiEndpoint)
	}

	if p.dnsConfig != nil {
		err := p.startDNS()
		p.dnsStatus.record(p.clock(), err)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Plane) serve(
	listener net.Listener,
	address string,
) {
	if p.onServeResult != nil {
		p.onServeResult(nil)
	}

	server := p.server
	go func() {
		err := server.Serve(listener)

		if err != nil && err != http.ErrServerClosed {
			// not fatal — the device is still up
			p.log.Error(
				"api server exited",
				"addr",
				address,
				"err",
				err,
			)
			if p.onServeResult != nil {
				p.onServeResult(err)
			}
		}
	}()
}

// stop shuts down DNS and HTTP (with bounded timeouts) then closes
// the WireGuard device. Errors are joined.
func (p *Plane) stop() error {
	var errs []error

	if err := p.stopDNS(); err != nil {
		errs = append(errs, err)
	}

	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		err := p.server.Shutdown(ctx)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"server shutdown %q: %w",
				p.config.Name,
				err,
			))
			if err := p.server.Close(); err != nil {
				errs = append(errs, fmt.Errorf(
					"server force close %q: %w",
					p.config.Name,
					err,
				))
			}
		}

		// Forget the server either way, so a later stop is a no-op.
		p.server = nil
	}

	if p.device != nil {
		if err := p.device.Close(); err != nil {
			errs = append(errs, fmt.Errorf(
				"device close %q: %w",
				p.config.Name,
				err,
			))
		}
		p.device = nil
	}

	return errors.Join(errs...)
}

// check repairs DNS without replacing the device or HTTP server.
// Network serializes checks and status reads, and retires checks before shutdown.
func (p *Plane) check() error {
	if p.dnsConfig == nil {
		return nil
	}

	if p.dns != nil && p.dns.Status().Serving {
		p.dnsStatus.record(p.clock(), nil)
		return nil
	}

	if err := p.stopDNS(); err != nil {
		p.dnsStatus.record(p.clock(), err)
		return err
	}

	err := p.startDNS()
	p.dnsStatus.record(p.clock(), err)
	if err != nil {
		return err
	}

	p.log.Info("dns repaired", "address", p.dns.Address())

	return nil
}

// startDNS constructs DNS on this plane's interface address.
func (p *Plane) startDNS() error {
	route, err := netaddr.InterfaceRouteFromCidr(p.config.Cidr)
	if err != nil {
		return fmt.Errorf("dns address: %w", err)
	}

	port := p.dnsConfig.port
	if port == 0 {
		port = 53
	}

	p.dns, err = dns.New(dns.Options{
		Network: p.dnsConfig.network,
		Zone:    p.dnsConfig.network + ".internal",
		Address: netaddr.Endpoint(route.IP, port),
		Service: p.dnsConfig.service,
		Logger:  p.log,
	})
	if err != nil {
		return fmt.Errorf("dns: %w", err)
	}

	if err := p.dns.Start(p.ctx); err != nil {
		return fmt.Errorf("dns: %w", err)
	}

	p.log.Debug("dns listening", "addr", p.dns.Address())

	return nil
}

// stopDNS gives DNS a bounded grace period; Stop owns forced cleanup.
func (p *Plane) stopDNS() error {
	if p.dns == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	err := p.dns.Stop(ctx)
	cancel()
	p.dns = nil
	if err != nil {
		return fmt.Errorf("dns stop %q: %w", p.config.Name, err)
	}
	return nil
}

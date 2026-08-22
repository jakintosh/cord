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
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// shutdownTimeout bounds the graceful shutdown of a plane's API server.
const shutdownTimeout = 5 * time.Second

// Plane is a running WireGuard plane: one WG device plus an optional
// HTTP API server served over the tunnel.
type Plane struct {
	device        *wireguard.Device
	server        *http.Server
	config        service.Plane
	privateKey    string
	log           *slog.Logger
	onServeResult func(error)
}

// start creates the WireGuard device, then synchronously starts the HTTP
// API listener (if handler is non-nil), so a bind failure fails the
// start instead of surfacing asynchronously. Requests served by the
// listener inherit ctx.
func (p *Plane) start(
	ctx context.Context,
	wg *wireguard.Manager,
	handler http.Handler,
) error {
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
		p.server = &http.Server{
			Addr:        apiEndpoint,
			Handler:     handler,
			BaseContext: func(net.Listener) context.Context { return ctx },
		}
		ln, err := net.Listen("tcp", apiEndpoint)
		if err != nil {
			_ = p.device.Close()
			return fmt.Errorf("listen %q on %s: %w", p.config.Name, apiEndpoint, err)
		}
		p.log.Debug("api listening", "addr", apiEndpoint)
		p.serve(ln, apiEndpoint)
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
	go func() {
		err := p.server.Serve(listener)

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

// stop shuts down the API server (with a bounded timeout) then closes
// the WireGuard device. Errors are joined.
func (p *Plane) stop() error {
	var errs []error

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

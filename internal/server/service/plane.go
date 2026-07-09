package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// PlaneConfig is the persisted configuration for one WireGuard plane
// (main or invite) within a network. It describes the interface name,
// address space, and port assignments.
type PlaneConfig struct {
	Name          string // WireGuard interface name
	Cidr          string // e.g. "10.42.0.0/16"
	WireguardPort uint16
	ApiPort       uint16
}

// validate checks that a single plane config has a valid device name
// and CIDR.
func (pc *PlaneConfig) validate() error {
	if err := wireguard.ValidateDeviceName(pc.Name); err != nil {
		return fmt.Errorf("%w: invalid device name: %v", ErrInvalidInput, err)
	}
	if _, _, err := net.ParseCIDR(pc.Cidr); err != nil {
		return fmt.Errorf("%w: invalid CIDR %q: %v", ErrInvalidInput, pc.Cidr, err)
	}
	return nil
}

// Plane is a running WireGuard plane: one WG device plus an optional
// HTTP API server served over the tunnel.
type Plane struct {
	device     *wireguard.Device
	server     *http.Server
	config     *PlaneConfig
	privateKey string
	log        *slog.Logger
}

func newPlane(
	config PlaneConfig,
	privateKey string,
	log *slog.Logger,
) *Plane {
	return &Plane{
		config:     &config,
		privateKey: privateKey,
		log:        log,
	}
}

// start creates the WireGuard device, then synchronously starts the
// HTTP API listener (if handler is non-nil). Bind failures now fail
// startup instead of being logged asynchronously.
func (p *Plane) start(
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
			Addr:    apiEndpoint,
			Handler: handler,
		}
		ln, err := net.Listen("tcp", apiEndpoint)
		if err != nil {
			_ = p.device.Close()
			return fmt.Errorf("listen %q on %s: %w", p.config.Name, apiEndpoint, err)
		}
		p.log.Debug("api listening", "addr", apiEndpoint)
		go func() {
			if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				// not fatal — the device is still up
				p.log.Error("api server exited", "addr", apiEndpoint, "err", err)
			}
		}()
	}

	return nil
}

// stop shuts down the API server (with a 5s timeout) then closes the
// WireGuard device. Errors are joined.
func (p *Plane) stop() error {
	var errs []error

	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := p.server.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("server shutdown %q: %w", p.config.Name, err))
		}
		cancel()
	}

	if p.device != nil {
		if err := p.device.Close(); err != nil {
			errs = append(errs, fmt.Errorf("device close %q: %w", p.config.Name, err))
		}
	}

	return errors.Join(errs...)
}

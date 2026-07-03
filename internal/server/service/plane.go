package service

import (
	"context"
	"errors"
	"fmt"
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

func (cfg PlaneConfig) toWireguardConfig(
	privateKey string,
) wireguard.DeviceConfig {
	_, net, err := net.ParseCIDR(cfg.Cidr)
	if err != nil {
		return wireguard.DeviceConfig{}
	}
	addr := netaddr.InterfaceAddress(net)

	return wireguard.DeviceConfig{
		Name:       cfg.Name,
		PrivateKey: privateKey,
		Address:    addr,
		ListenPort: cfg.WireguardPort,
	}
}

// Plane is a running WireGuard plane: one WG device plus an optional
// HTTP API server served over the tunnel.
type Plane struct {
	device     *wireguard.Device
	server     *http.Server
	config     *PlaneConfig
	privateKey string
}

func newPlane(
	config PlaneConfig,
	privateKey string,
) *Plane {
	return &Plane{
		config:     &config,
		privateKey: privateKey,
	}
}

// start creates the WireGuard device, then synchronously starts the
// HTTP API listener (if handler is non-nil). Bind failures now fail
// startup instead of being logged asynchronously.
func (p *Plane) start(
	wg *wireguard.Manager,
	handler http.Handler,
) error {
	cfg := p.config.toWireguardConfig(p.privateKey)
	dev, err := wg.CreateDevice(cfg)
	if err != nil {
		return fmt.Errorf("create device %q: %w", p.config.Name, err)
	}
	p.device = dev

	if handler != nil {
		ifaceIP := netaddr.FirstAssignable(&cfg.Address)
		addr := netaddr.Endpoint(ifaceIP, p.config.ApiPort)
		p.server = &http.Server{
			Addr:    addr,
			Handler: handler,
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			_ = p.device.Close()
			return fmt.Errorf("listen %q on %s: %w", p.config.Name, addr, err)
		}
		go func() {
			if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				// TODO: logged but not fatal — the device is still up
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

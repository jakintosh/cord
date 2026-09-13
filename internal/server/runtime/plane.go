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
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/dns"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const defaultShutdownTimeout = 5 * time.Second

// PlaneStatus is a snapshot of the plane's configured child resources.
type PlaneStatus struct {
	WireGuard resource.Status
	DNS       resource.Status
	HTTP      resource.Status
}

// Plane groups a device and its servers. Only the runtime loop operates on it.
type Plane struct {
	device *wireguard.Device
	http   *httpServer
	dns    *dns.Server

	timeout time.Duration
}

// newPlane constructs configured wrappers without starting any resources.
func newPlane(
	config service.Plane,
	dnsOptions *dns.Options,
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
	if dnsOptions != nil {
		dnsSrv, err = dns.New(*dnsOptions)
		if err != nil {
			return nil, fmt.Errorf("dns: %w", err)
		}
	}

	return &Plane{
		device:  device,
		http:    httpSrv,
		dns:     dnsSrv,
		timeout: timeout,
	}, nil
}

// status collects child status without changing resources.
func (p *Plane) status() PlaneStatus {
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
		status.HTTP = p.http.status()
	}

	return status
}

// reconcile is infrastructure maintenance, never a plane-handler operation.
func (p *Plane) reconcile(
	ctx context.Context,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var errs []error
	errs = append(errs, p.device.Reconcile(ctx))
	if p.dns != nil {
		errs = append(errs, p.dns.Reconcile(ctx))
	}
	if p.http != nil {
		errs = append(errs, p.http.reconcile(ctx))
	}
	return errors.Join(errs...)
}

// close releases request waiters and drains servers before closing the device.
func (p *Plane) close() error {
	var errs []error
	if p.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		errs = append(errs, p.http.close(ctx))
		cancel()
	}

	if p.dns != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		errs = append(errs, p.dns.Close(ctx))
		cancel()
	}

	errs = append(errs, p.device.Close())
	return errors.Join(errs...)
}

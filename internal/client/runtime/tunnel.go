package runtime

import (
	"context"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

type tunnelOptions struct {
	name       string
	privateKey string
	route      string
	listenPort uint16
	server     service.ServerInfo
	keepalive  time.Duration
}

// Tunnel owns a configured device and the coordination server's API address.
// Its parent serializes lifecycle calls and owns cleanup after failed startup.
type Tunnel struct {
	device      *wireguard.Device
	apiAddr     string
	serverPeer  wireguard.PeerConfig
	initialized bool
}

// newTunnel constructs a device wrapper without creating an interface.
func newTunnel(
	opts tunnelOptions,
	wg *wireguard.Manager,
) (
	*Tunnel,
	error,
) {
	deviceRoute, err := netaddr.ParseRoute(opts.route)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: parse route %q",
			service.ErrInvalidInput,
			opts.route,
		)
	}

	_, networkCIDR, err := net.ParseCIDR(opts.server.NetworkCidr)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: parse network cidr %q",
			service.ErrInvalidInput,
			opts.server.NetworkCidr,
		)
	}

	apiAddr, err := netaddr.EndpointFromCIDR(opts.server.Route, opts.server.APIPort)
	if err != nil {
		return nil, fmt.Errorf("server api addr for %q: %w", opts.name, err)
	}

	device, err := wg.NewDevice(wireguard.DeviceConfig{
		Name:        opts.name,
		PrivateKey:  opts.privateKey,
		Route:       deviceRoute,
		NetworkCIDR: *networkCIDR,
		ListenPort:  opts.listenPort,
	})
	if err != nil {
		return nil, fmt.Errorf("create device %q: %w", opts.name, err)
	}

	serverPeer := wireguard.PeerConfig{
		PublicKey:           opts.server.PublicKey,
		AllowedIPs:          []string{opts.server.Route},
		Endpoint:            opts.server.Endpoint,
		EndpointPolicy:      wireguard.EndpointFixed,
		PersistentKeepalive: int(opts.keepalive / time.Second),
	}
	if _, err := serverPeer.Parse(); err != nil {
		return nil, fmt.Errorf("server peer for %q: %w", opts.name, err)
	}

	return &Tunnel{
		device:     device,
		apiAddr:    apiAddr,
		serverPeer: serverPeer,
	}, nil
}

// status observes device availability without repairing it.
func (t *Tunnel) status() resource.Status {
	return t.device.Status()
}

// reconcile restores the device and seeds its server peer once. Subsequent
// repairs preserve the full desired peer set maintained by the network.
func (t *Tunnel) reconcile(
	ctx context.Context,
) error {
	if err := t.device.Reconcile(ctx); err != nil {
		return err
	}

	if !t.initialized {
		if err := t.device.SetPeers(t.serverPeer); err != nil {
			return fmt.Errorf("set server peer: %w", err)
		}
		t.initialized = true
	}

	return nil
}

// close releases the interface, retaining the wrapper for retries and restarts.
func (t *Tunnel) close() error {
	if t.device == nil {
		return nil
	}

	return t.device.Close()
}

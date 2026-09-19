package runtime

import (
	"context"
	"fmt"
	"net"

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
}

// Tunnel owns a configured device and the coordination server's API address.
// Its parent configures peers, serializes lifecycle calls, and owns cleanup.
type Tunnel struct {
	name    string
	device  *wireguard.Device
	apiAddr string
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

	return &Tunnel{
		name:    opts.name,
		device:  device,
		apiAddr: apiAddr,
	}, nil
}

// status observes device availability without repairing it.
func (t *Tunnel) status(
	target resource.State,
) ResourceSnapshot {
	device := t.device.Status()

	child := ResourceSnapshot{
		Kind:   "wireguard-device",
		Name:   t.name,
		State:  device.State,
		Target: target,
	}
	if device.Error != nil {
		child.Error = device.Error.Error()
	}
	child.updateHealth()

	status := ResourceSnapshot{
		Kind:     "tunnel",
		Name:     t.name,
		State:    device.State,
		Target:   target,
		Children: []ResourceSnapshot{child},
	}
	status.updateHealth()

	return status
}

// reconcile restores device availability. The device retains and restores
// the desired peers configured by the tunnel's parent.
func (t *Tunnel) reconcile(
	ctx context.Context,
) error {
	return t.device.Reconcile(ctx)
}

// close releases the interface, retaining the wrapper for retries and restarts.
func (t *Tunnel) close() error {
	if t.device == nil {
		return nil
	}

	return t.device.Close()
}

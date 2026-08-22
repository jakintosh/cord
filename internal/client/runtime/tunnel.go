package runtime

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Tunnel is a running WireGuard device with the coordination server
// pinned as its one fixed peer, plus the address its API answers on.
// Every call this daemon makes to a server travels through one.
type Tunnel struct {
	device  *wireguard.Device
	apiAddr string // server API host:port derived from server

	name       string
	privateKey string
	route      string
	listenPort uint16
	server     service.ServerInfo
	keepalive  time.Duration
}

// start creates the device and adds the server peer with EndpointFixed.
// The caller is responsible for calling stop when done.
func (t *Tunnel) start(
	wg *wireguard.Manager,
) error {
	deviceRoute, err := netaddr.ParseRoute(t.route)
	if err != nil {
		return fmt.Errorf(
			"%w: parse route %q",
			service.ErrInvalidInput,
			t.route,
		)
	}

	_, networkCIDR, err := net.ParseCIDR(t.server.NetworkCidr)
	if err != nil {
		return fmt.Errorf(
			"%w: parse network cidr %q",
			service.ErrInvalidInput,
			t.server.NetworkCidr,
		)
	}

	t.device, err = wg.CreateDevice(wireguard.DeviceConfig{
		Name:        t.name,
		PrivateKey:  t.privateKey,
		Route:       deviceRoute,
		NetworkCIDR: *networkCIDR,
		ListenPort:  t.listenPort,
	})
	if err != nil {
		return fmt.Errorf("create device %q: %w", t.name, err)
	}

	if err := t.device.SetPeers(wireguard.PeerConfig{
		PublicKey:           t.server.PublicKey,
		AllowedIPs:          []string{t.server.Route},
		Endpoint:            t.server.Endpoint,
		EndpointPolicy:      wireguard.EndpointFixed,
		PersistentKeepalive: int(t.keepalive / time.Second),
	}); err != nil {
		_ = t.device.Close()
		return fmt.Errorf("set server peer on %q: %w", t.name, err)
	}

	t.apiAddr, err = netaddr.EndpointFromCIDR(t.server.Route, t.server.APIPort)
	if err != nil {
		_ = t.device.Close()
		return fmt.Errorf("server api addr for %q: %w", t.name, err)
	}

	return nil
}

// stop closes the WireGuard device.
func (t *Tunnel) stop() error {
	if t.device == nil {
		return nil
	}

	err := t.device.Close()
	t.device = nil

	return err
}

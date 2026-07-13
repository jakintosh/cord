package service

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Tunnel owns one WireGuard device with a pinned server peer.
type Tunnel struct {
	device  *wireguard.Device
	apiAddr string // server API host:port derived from ServerInfo
}

// newTunnel creates a device, adds the server peer with EndpointFixed,
// and returns a ready Tunnel. The caller is responsible for calling
// stop() when done.
func newTunnel(
	mgr *wireguard.Manager,
	ifaceName string,
	privateKey string,
	route string,
	server ServerInfo,
	listenPort uint16,
	persistentKeepalive time.Duration,
) (
	*Tunnel,
	error,
) {
	deviceRoute, err := netaddr.ParseRoute(route)
	if err != nil {
		return nil, fmt.Errorf("%w: parse route %q", ErrInvalidInput, route)
	}

	_, networkCIDR, err := net.ParseCIDR(server.NetworkCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: parse network cidr %q", ErrInvalidInput, server.NetworkCidr)
	}

	dev, err := mgr.CreateDevice(wireguard.DeviceConfig{
		Name:        ifaceName,
		PrivateKey:  privateKey,
		Route:       deviceRoute,
		NetworkCIDR: *networkCIDR,
		ListenPort:  listenPort,
	})
	if err != nil {
		return nil, fmt.Errorf("create device %q: %w", ifaceName, err)
	}

	if err := dev.SetPeers(wireguard.PeerConfig{
		PublicKey:           server.PublicKey,
		AllowedIPs:          []string{server.Route},
		Endpoint:            server.Endpoint,
		EndpointPolicy:      wireguard.EndpointFixed,
		PersistentKeepalive: int(persistentKeepalive / time.Second),
	}); err != nil {
		_ = dev.Close()
		return nil, fmt.Errorf("set server peer on %q: %w", ifaceName, err)
	}

	apiAddr, err := netaddr.EndpointFromCIDR(server.Route, server.APIPort)
	if err != nil {
		_ = dev.Close()
		return nil, fmt.Errorf("server api addr for %q: %w", ifaceName, err)
	}

	return &Tunnel{
		device:  dev,
		apiAddr: apiAddr,
	}, nil
}

// stop closes the WireGuard device.
func (t *Tunnel) stop() error {
	if t.device != nil {
		err := t.device.Close()
		t.device = nil
		return err
	}
	return nil
}

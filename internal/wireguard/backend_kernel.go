//go:build linux

package wireguard

import (
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// KernelBackend implements Backend for Linux using the kernel
// WireGuard module.
type KernelBackend struct{}

func (b *KernelBackend) CreateDevice(
	cfg DeviceConfig,
) (
	WgDevice,
	error,
) {
	link, err := kernelEnsureLink(cfg.Name)
	if err != nil {
		return nil, err
	}

	if err := kernelSyncAddress(link, cfg.Route); err != nil {
		return nil, err
	}

	mtu := cfg.MTU
	if mtu <= 0 {
		mtu = defaultMTU
	}
	if err := netlink.LinkSetMTU(link, mtu); err != nil {
		return nil, fmt.Errorf("wireguard: set mtu %s: %w", cfg.Name, err)
	}

	privKey, err := parseKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("wireguard: parse key: %w", err)
	}

	if err := kernelApplyDeviceConfig(cfg.Name, privKey, int(cfg.ListenPort)); err != nil {
		return nil, err
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("wireguard: bring %s up: %w", cfg.Name, err)
	}

	// WireGuard's kernel AllowedIPs select a peer for packets already
	// routed to this interface; they do not create routes in the host FIB.
	// Install the overlay route so traffic to the server and other peers
	// reaches this device, matching the userspace Linux backend.
	if err := kernelSyncNetworkRoute(link, cfg.NetworkCIDR); err != nil {
		return nil, err
	}

	return &kernelDeviceHandle{name: cfg.Name}, nil
}

// kernelDeviceHandle is a WgDevice backed by the kernel WireGuard
// module. It holds the interface link name.
type kernelDeviceHandle struct {
	name string
}

func (h *kernelDeviceHandle) Name() string {
	return h.name
}

func (h *kernelDeviceHandle) Peers() (
	[]PeerStatus,
	error,
) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("wireguard: wgctrl: %w", err)
	}
	defer client.Close()

	dev, err := client.Device(h.name)
	if err != nil {
		return nil, fmt.Errorf("wireguard: query %s: %w", h.name, err)
	}

	peers := make([]PeerStatus, len(dev.Peers))
	for i, p := range dev.Peers {
		peers[i] = PeerStatus{
			PublicKey:           p.PublicKey,
			AllowedIPs:          p.AllowedIPs,
			Endpoint:            p.Endpoint,
			PersistentKeepalive: p.PersistentKeepaliveInterval,
			LastHandshake:       p.LastHandshakeTime,
			ReceiveBytes:        p.ReceiveBytes,
			TransmitBytes:       p.TransmitBytes,
		}
	}
	return peers, nil
}

func (h *kernelDeviceHandle) ApplyPeers(
	ops []PeerOp,
) error {
	link, err := netlink.LinkByName(h.name)
	if err != nil {
		return fmt.Errorf("wireguard: get link %s: %w", h.name, err)
	}

	if link.Attrs().Flags&net.FlagUp == 0 {
		return fmt.Errorf("wireguard: %s is not up", h.name)
	}

	return kernelApplyPeerOperations(h.name, ops)
}

func (h *kernelDeviceHandle) Close() error {
	link, err := netlink.LinkByName(h.name)
	if err != nil {
		if _, ok := errors.AsType[netlink.LinkNotFoundError](err); ok {
			return nil
		}
		return fmt.Errorf("wireguard: get link %s: %w", h.name, err)
	}
	downErr := netlink.LinkSetDown(link)
	deleteErr := netlink.LinkDel(link)
	if deleteErr == nil {
		return nil
	}

	deleteErr = fmt.Errorf("wireguard: delete %s: %w", h.name, deleteErr)
	if downErr == nil {
		return deleteErr
	}
	return errors.Join(
		fmt.Errorf("wireguard: bring down %s: %w", h.name, downErr),
		deleteErr,
	)
}

func kernelApplyPeerOperations(
	name string,
	operations []PeerOp,
) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wireguard: wgctrl: %w", err)
	}
	defer client.Close()

	peerConfigs := make([]wgtypes.PeerConfig, 0, len(operations))
	for _, op := range operations {
		peerConfigs = append(peerConfigs, op.toWgPeerConfig())
	}

	return client.ConfigureDevice(name, wgtypes.Config{Peers: peerConfigs})
}

func kernelEnsureLink(
	name string,
) (
	netlink.Link,
	error,
) {
	link, err := netlink.LinkByName(name)

	// if link found
	if err == nil {
		return link, nil
	}

	// if link errored, but not with notFound error
	var notFound netlink.LinkNotFoundError
	if !errors.As(err, &notFound) {
		return nil, fmt.Errorf("wireguard: get link %s: %w", name, err)
	}

	// link not found, create it

	attr := netlink.NewLinkAttrs()
	attr.Name = name

	wg := &netlink.Wireguard{LinkAttrs: attr}
	if err := netlink.LinkAdd(wg); err != nil {
		return nil, fmt.Errorf("wireguard: create %s: %w", name, err)
	}

	return wg, nil
}

// kernelSyncAddress replaces the addresses on link with route. The route is
// deliberately normally a /32: it identifies this WireGuard device, while
// kernelSyncNetworkRoute supplies the overlay network route.
func kernelSyncAddress(
	link netlink.Link,
	addr net.IPNet,
) error {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("wireguard: list addrs: %w", err)
	}

	desired := &netlink.Addr{IPNet: &addr}
	found := false
	for _, existing := range addrs {
		if existing.Equal(*desired) {
			found = true
			continue
		}
		if err := netlink.AddrDel(link, &existing); err != nil {
			return fmt.Errorf("wireguard: del addr %s: %w", existing.IPNet.String(), err)
		}
	}

	if !found {
		if err := netlink.AddrAdd(link, desired); err != nil {
			return fmt.Errorf("wireguard: add addr %s: %w", addr.String(), err)
		}
	}

	return nil
}

// kernelSyncNetworkRoute sends the configured overlay CIDR through link.
// Unlike wg-quick, the kernel WireGuard API does not add routes from a peer's
// AllowedIPs, so this is required for inner traffic to reach the device.
func kernelSyncNetworkRoute(
	link netlink.Link,
	networkCIDR net.IPNet,
) error {
	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       &networkCIDR,
	}
	if err := netlink.RouteReplace(route); err != nil {
		return fmt.Errorf("wireguard: add route for %s: %w", networkCIDR.String(), err)
	}
	return nil
}

func kernelApplyDeviceConfig(
	name string,
	key wgtypes.Key,
	port int,
) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wireguard: wgctrl: %w", err)
	}
	defer client.Close()

	cfg := wgtypes.Config{PrivateKey: &key}
	if port > 0 {
		cfg.ListenPort = &port
	}

	if err := client.ConfigureDevice(name, cfg); err != nil {
		return fmt.Errorf("wireguard: configure %s: %w", name, err)
	}
	return nil
}

func (op PeerOp) toWgPeerConfig() wgtypes.PeerConfig {
	if op.Remove {
		return wgtypes.PeerConfig{
			PublicKey: op.Target.PublicKey,
			Remove:    true,
		}
	}
	cfg := wgtypes.PeerConfig{
		PublicKey:         op.Target.PublicKey,
		ReplaceAllowedIPs: true,
		AllowedIPs:        op.Target.AllowedIPs,
	}
	keepalive := op.Target.PersistentKeepalive
	cfg.PersistentKeepaliveInterval = &keepalive
	if op.Target.Endpoint != nil {
		cfg.Endpoint = op.Target.Endpoint
	}
	return cfg
}

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

	if err := kernelSyncRoute(link, cfg.Route); err != nil {
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

	return &kernelDeviceHandle{name: cfg.Name}, nil
}

// kernelDeviceHandle is a WgDevice backed by the kernel WireGuard
// module. It holds the interface link name.
type kernelDeviceHandle struct {
	name string
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
	_ = netlink.LinkSetDown(link)
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("wireguard: delete %s: %w", h.name, err)
	}
	return nil
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

func kernelSyncRoute(
	link netlink.Link,
	route net.IPNet,
) error {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("wireguard: list addrs: %w", err)
	}

	desired := &netlink.Addr{IPNet: &route}
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
			return fmt.Errorf("wireguard: add addr %s: %w", route.String(), err)
		}
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
			PublicKey: op.Config.PublicKey,
			Remove:    true,
		}
	}
	cfg := wgtypes.PeerConfig{
		PublicKey:         op.Config.PublicKey,
		ReplaceAllowedIPs: true,
		AllowedIPs:        op.Config.AllowedIPs,
	}
	keepalive := op.Config.PersistentKeepalive
	cfg.PersistentKeepaliveInterval = &keepalive
	if op.Config.Endpoint != nil {
		cfg.Endpoint = op.Config.Endpoint
	}
	return cfg
}

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

func (b *KernelBackend) Up(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	listenPort int,
	mtu int,
	noRoutes bool,
) error {
	link, err := ensureLink(name)
	if err != nil {
		return err
	}

	if err := syncAddress(link, address); err != nil {
		return err
	}

	if mtu <= 0 {
		mtu = defaultMTU
	}
	if err := netlink.LinkSetMTU(link, mtu); err != nil {
		return fmt.Errorf("wireguard: set mtu %s: %w", name, err)
	}

	if err := applyDeviceConfig(name, privateKey, listenPort); err != nil {
		return err
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("wireguard: bring %s up: %w", name, err)
	}

	return nil
}

func (b *KernelBackend) Down(
	name string,
) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		var notFound netlink.LinkNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("wireguard: get link %s: %w", name, err)
	}

	if err := netlink.LinkSetDown(link); err != nil {
		return fmt.Errorf("wireguard: bring %s down: %w", name, err)
	}

	return nil
}

func (b *KernelBackend) Delete(
	name string,
) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		var notFound netlink.LinkNotFoundError
		if errors.As(err, &notFound) {
			return nil
		}
		return fmt.Errorf("wireguard: get link %s: %w", name, err)
	}

	_ = netlink.LinkSetDown(link)

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("wireguard: delete %s: %w", name, err)
	}

	return nil
}

func (b *KernelBackend) GetPeers(
	name string,
) (
	[]Peer,
	error,
) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("wireguard: wgctrl: %w", err)
	}
	defer client.Close()

	dev, err := client.Device(name)
	if err != nil {
		return nil, fmt.Errorf("wireguard: query %s: %w", name, err)
	}

	peers := make([]Peer, len(dev.Peers))
	for i, p := range dev.Peers {
		peers[i] = Peer{
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

func (b *KernelBackend) ModifyPeers(
	name string,
	operations []PeerOperation,
) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("wireguard: get link %s: %w", name, err)
	}

	if link.Attrs().Flags&net.FlagUp == 0 {
		return fmt.Errorf("wireguard: %s is not up", name)
	}

	return kernelApplyPeerOperations(name, operations)
}

func kernelApplyPeerOperations(
	name string,
	operations []PeerOperation,
) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wireguard: wgctrl: %w", err)
	}
	defer client.Close()

	peerConfigs := make([]wgtypes.PeerConfig, 0, len(operations))
	for _, op := range operations {
		peerConfigs = append(peerConfigs, wgPeerConfig(op))
	}

	return client.ConfigureDevice(name, wgtypes.Config{Peers: peerConfigs})
}

func ensureLink(
	name string,
) (
	netlink.Link,
	error,
) {
	link, err := netlink.LinkByName(name)
	if err == nil {
		return link, nil
	}

	var notFound netlink.LinkNotFoundError
	if !errors.As(err, &notFound) {
		return nil, fmt.Errorf("wireguard: get link %s: %w", name, err)
	}

	attr := netlink.NewLinkAttrs()
	attr.Name = name

	wg := &netlink.Wireguard{LinkAttrs: attr}
	if err := netlink.LinkAdd(wg); err != nil {
		return nil, fmt.Errorf("wireguard: create %s: %w", name, err)
	}

	return wg, nil
}

func syncAddress(
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

func applyDeviceConfig(
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

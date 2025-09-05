//go:build linux

package wireguard

import (
	"fmt"
	"net"
	"os"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// KernelBackend implements the Backend interface for Linux systems using
// the kernel WireGuard implementation. This is the default implementation
// on Linux systems.
type KernelBackend struct{}

// Backend interface implementation

// Up creates the network device if it doesn't exist, configures it with the
// current state of the Interface object, and brings it up.
// It also writes the native .conf file to the specified path.
func (b *KernelBackend) Up(
	iface *Interface,
	configPath string,
) error {
	// Ensure the link exists
	link, err := ensureLink(iface.Name)
	if err != nil {
		return err
	}

	// Sync the address
	err = syncAddress(link, iface.Address)
	if err != nil {
		return err
	}

	// Apply device configuration (private key, listen port)
	err = applyDeviceConfig(iface.Name, iface.PrivateKey, iface.ListenPort)
	if err != nil {
		return err
	}

	// Apply peers
	err = applyPeers(iface.Name, iface.Peers)
	if err != nil {
		return err
	}

	// Bring the link up
	err = netlink.LinkSetUp(link)
	if err != nil {
		return fmt.Errorf("failed to bring interface up: %w", err)
	}

	// Write the native config file
	if configPath != "" {
		config := iface.ToWgConfig()
		err = os.WriteFile(configPath, []byte(config), 0600)
		if err != nil {
			return fmt.Errorf("failed to write config file %s: %w", configPath, err)
		}
	}

	return nil
}

// Down brings the interface down and, optionally, deletes it.
func (b *KernelBackend) Down(
	iface *Interface,
	delete bool,
) error {
	// Get the link - if it doesn't exist, return success
	link, err := netlink.LinkByName(iface.Name)
	if link == nil {
		// Interface doesn't exist, nothing to do
		return nil
	}
	if err != nil {
		// error getting interface, surface err
		return fmt.Errorf("failed to get link named %s: %w", iface.Name, err)
	}

	// Bring the link down
	err = netlink.LinkSetDown(link)
	if err != nil {
		return fmt.Errorf("failed to bring interface down: %w", err)
	}

	// Delete the interface if requested
	if delete {
		err = netlink.LinkDel(link)
		if err != nil {
			return fmt.Errorf("failed to delete interface: %w", err)
		}
	}

	return nil
}

// Sync applies only the changes to the peer list to a live interface
// without tearing it down. This is more efficient for updates.
func (b *KernelBackend) Sync(
	iface *Interface,
) error {
	// Get the link - if it doesn't exist or is not up, return an error
	link, err := netlink.LinkByName(iface.Name)
	if link == nil {
		return fmt.Errorf("interface %s does not exist: %w", iface.Name, err)
	}
	if err != nil {
		return fmt.Errorf("failed to get link named %s: %w", iface.Name, err)
	}

	// Check if the interface is up
	if link.Attrs().Flags&net.FlagUp == 0 {
		return fmt.Errorf("interface %s is not up", iface.Name)
	}

	// Apply only the peers
	err = applyPeers(iface.Name, iface.Peers)
	if err != nil {
		return err
	}

	return nil
}

func ensureLink(
	name string,
) (
	netlink.Link,
	error,
) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get link named %s: %w", name, err)
	}

	// if link exists, return it immediately
	if link != nil {
		return link, nil
	}

	// If link doesn't exist, create it

	// create netlink attributes
	attr := netlink.NewLinkAttrs()
	attr.Name = name

	// create wireguard netlink descriptor
	wg := &netlink.Wireguard{LinkAttrs: attr}

	// add wg link
	err = netlink.LinkAdd(wg)
	if err != nil {
		return nil, fmt.Errorf("failed to create WireGuard interface %s: %w", name, err)
	}

	return wg, nil
}

// syncAddress ensures the interface has exactly the specified IP address,
// adding it if missing and removing any others.
func syncAddress(
	link netlink.Link,
	addr net.IPNet,
) error {
	// Get current addresses
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list addresses: %w", err)
	}

	// Check if our desired address is already present
	desired := &netlink.Addr{IPNet: &addr}
	found := false
	for _, existing := range addrs {
		if existing.Equal(*desired) {
			found = true
			continue
		}

		// delete any addr that is not our desired addr
		err = netlink.AddrDel(link, &existing)
		if err != nil {
			return fmt.Errorf("failed to remove address %s: %w", existing.IPNet.String(), err)
		}
	}

	// Add the address if it's not present
	if !found {
		err = netlink.AddrAdd(link, desired)
		if err != nil {
			return fmt.Errorf("failed to add address %s: %w", addr.String(), err)
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
		return fmt.Errorf("failed to create WireGuard client: %w", err)
	}
	defer client.Close()

	cfg := wgtypes.Config{
		PrivateKey: &key,
	}

	if port > 0 {
		cfg.ListenPort = &port
	}

	err = client.ConfigureDevice(name, cfg)
	if err != nil {
		return fmt.Errorf("failed to configure device %s: %w", name, err)
	}

	return nil
}

func applyPeers(
	name string,
	peers []Peer,
) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to create WireGuard client: %w", err)
	}
	defer client.Close()

	// Convert our Peer structs to wgtypes.PeerConfig
	var peerConfigs []wgtypes.PeerConfig
	for _, peer := range peers {
		var allowedIPs []net.IPNet
		for _, ip := range peer.AllowedIPs {
			allowedIPs = append(allowedIPs, *ip)
		}

		peerConfig := wgtypes.PeerConfig{
			PublicKey:  peer.PublicKey,
			AllowedIPs: allowedIPs,
		}

		if peer.Endpoint != nil {
			peerConfig.Endpoint = peer.Endpoint
		}

		if peer.PersistentKeepalive > 0 {
			peerConfig.PersistentKeepaliveInterval = &peer.PersistentKeepalive
		}

		peerConfigs = append(peerConfigs, peerConfig)
	}

	cfg := wgtypes.Config{
		ReplacePeers: true,
		Peers:        peerConfigs,
	}

	err = client.ConfigureDevice(name, cfg)
	if err != nil {
		return fmt.Errorf("failed to configure peers for device %s: %w", name, err)
	}

	return nil
}

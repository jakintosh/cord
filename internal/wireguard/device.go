package wireguard

import (
	"fmt"
	"io"
	"net"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type DeviceConfig struct {
	PrivateKey wgtypes.Key // wg private key
	Cidr       *net.IPNet  // internal ip / network mask
	ListenPort uint16      // external listening port
}

func NewDeviceConfig(
	privateKey wgtypes.Key,
	networkCidr *net.IPNet,
	address net.IP,
	port uint16,
) (
	*DeviceConfig,
	error,
) {
	if !networkCidr.Contains(address) {
		return nil, fmt.Errorf(
			"address '%s' is not within cidr '%s'",
			address.String(), networkCidr.String(),
		)
	}
	return &DeviceConfig{
		PrivateKey: privateKey,
		Cidr: &net.IPNet{
			IP:   address,
			Mask: networkCidr.Mask,
		},
		ListenPort: port,
	}, nil
}

func (c *DeviceConfig) Write(
	w io.Writer,
) error {
	_, err := fmt.Fprintf(w,
		"private-key = \"%s\"\ncidr = \"%s\"\nlisten-port = %d\n",
		c.PrivateKey.String(), c.Cidr, c.ListenPort,
	)
	return err
}

func listDevices() {
	client, err := wgctrl.New()
	if err != nil {
		fmt.Printf("failed to load wg client: %v\n", err)
		return
	}
	defer client.Close()

	devices, err := client.Devices()
	if err != nil {
		fmt.Printf("failed to get wg devices: %v", err)
		return
	}
	if len(devices) == 0 {
		fmt.Printf("no wg devices\n")
		return
	}
	for _, device := range devices {
		printDevice(device)
	}
}

func createDevice(ifname string, ip string) {
	fmt.Printf("new network\n")
	attr := netlink.NewLinkAttrs()
	attr.Name = ifname
	wg := &netlink.Wireguard{LinkAttrs: attr}

	addr, err := netlink.ParseAddr(ip)
	if err != nil {
		fmt.Printf("failed to parse ip '%s'\n", ip)
		return
	}

	err = netlink.AddrAdd(wg, addr)
	if err != nil {
		fmt.Printf("failed to add ip '%s' to %s\n", ip, ifname)
		return
	}

	err = netlink.LinkAdd(wg)
	if err != nil {
		fmt.Printf("failed to add wg link: %v\n", err)
		return
	}

	fmt.Printf("%+v\n", wg)

	client, err := wgctrl.New()
	if err != nil {
		fmt.Printf("failed to load wg client: %v\n", err)
		return
	}
	defer client.Close()

	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		fmt.Printf("Failed to generate private key: %v", err)
		return
	}

	port := 51820
	cfg := wgtypes.Config{
		PrivateKey: &key,
		ListenPort: &port,
		Peers:      []wgtypes.PeerConfig{},
	}

	err = client.ConfigureDevice(ifname, cfg)
	if err != nil {
		fmt.Printf("Failed to configure device: %v", err)
		return
	}

	fmt.Printf("WireGuard device %s created and configured successfully.\n", ifname)
}

func updateDevice(ifname string) {

}

func deleteDevice(ifname string) {
	attr := netlink.NewLinkAttrs()
	attr.Name = ifname
	wg := &netlink.Wireguard{LinkAttrs: attr}
	err := netlink.LinkDel(wg)
	if err != nil {
		fmt.Printf("failed to delete device: %v\n", err)
		return
	}
}

func printDevice(device *wgtypes.Device) {
	fmt.Printf("%s (:%d)\n", device.Name, device.ListenPort)
	for _, peer := range device.Peers {
		PrintPeer(&peer, 1)
	}
}

package app

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func printDevice(device *wgtypes.Device) {
	fmt.Printf("%s (:%d)\n", device.Name, device.ListenPort)
	for _, peer := range device.Peers {
		printPeer(&peer, 1)
	}
}

func printPeer(peer *wgtypes.Peer, indent int) {
	indents := strings.Repeat(" ", indent)
	ip := peer.AllowedIPs[0].IP
	pubKey := base64.RawStdEncoding.EncodeToString(peer.PublicKey[:])[:6]
	fmt.Printf("%s%s (%s...)\n", indents, ip, pubKey)
}

func listInterfaces() {
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

func generateKeypair() (private wgtypes.Key, public wgtypes.Key, err error) {
	private, err = wgtypes.GeneratePrivateKey()
	if err != nil {
		err = fmt.Errorf("failed to generate wg keys: %w", err)
		return
	}
	public = private.PublicKey()
	return
}

func createInterface(ifname string) {
	fmt.Printf("new network\n")
	attr := netlink.NewLinkAttrs()
	attr.Name = ifname
	wg := &netlink.Wireguard{LinkAttrs: attr}
	err := netlink.LinkAdd(wg)
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
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey:  key.PublicKey(), // Replace with actual public key
				AllowedIPs: []net.IPNet{{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}},
			},
		},
	}

	err = client.ConfigureDevice(ifname, cfg)
	if err != nil {
		fmt.Printf("Failed to configure device: %v", err)
		return
	}

	fmt.Printf("WireGuard device %s created and configured successfully.\n", ifname)
}

func deleteInterface(ifname string) {
	attr := netlink.NewLinkAttrs()
	attr.Name = ifname
	wg := &netlink.Wireguard{LinkAttrs: attr}
	err := netlink.LinkDel(wg)
	if err != nil {
		fmt.Printf("failed to delete device: %v\n", err)
		return
	}
}

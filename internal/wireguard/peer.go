package wireguard

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type PeerConfig struct {
	Name       string
	Ip         net.IP
	PrivateKey PrivateKey
}

func (c *PeerConfig) WriteInvite(
	path string,
) error {
	fmt.Printf(
		"Writing to: %s\n\n[Peer]\nname=%s\nip=%s\nprivateKey=%s",
		path, c.Name, c.Ip.String(), c.PrivateKey.String(),
	)
	return nil
}

func PrintPeer(peer *wgtypes.Peer, indent int) {
	indents := strings.Repeat(" ", indent)
	ip := peer.AllowedIPs[0].IP
	pubKey := base64.RawStdEncoding.EncodeToString(peer.PublicKey[:])[:6]
	fmt.Printf("%s%s (%s...)\n", indents, ip, pubKey)
}

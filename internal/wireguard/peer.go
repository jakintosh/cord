package wireguard

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type PeerConfig struct {
	Name       string
	Ip         net.IP
	PrivateKey PrivateKey
}

func (c *PeerConfig) Write(
	w io.Writer,
) error {
	_, err := fmt.Fprintf(w,
		"[Peer]\nname=%s\nip=%s\nprivateKey=%s",
		c.Name, c.Ip.String(), c.PrivateKey.String(),
	)
	return err
}

func PrintPeer(peer *wgtypes.Peer, indent int) {
	indents := strings.Repeat(" ", indent)
	ip := peer.AllowedIPs[0].IP
	pubKey := base64.RawStdEncoding.EncodeToString(peer.PublicKey[:])[:6]
	fmt.Printf("%s%s (%s...)\n", indents, ip, pubKey)
}

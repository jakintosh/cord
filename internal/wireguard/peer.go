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
	Cidr       *net.IPNet
	PrivateKey PrivateKey
}

func (c *PeerConfig) WriteConfig(
	w io.Writer,
) error {
	_, err := fmt.Fprintf(w,
		"[interface]\nnetwork-name=%s\naddress=%s\nprivate-key=%s\n\n[server]\npublic-key=%s\nexternal-endpoint=%s\ninternal-endpoint=%s",
		c.Name, c.Cidr.String(), c.PrivateKey.String(), "pubkey", "ext-end", "int-end",
	)
	return err
}

func PrintPeer(peer *wgtypes.Peer, indent int) {
	indents := strings.Repeat(" ", indent)
	ip := peer.AllowedIPs[0].IP
	pubKey := base64.RawStdEncoding.EncodeToString(peer.PublicKey[:])[:6]
	fmt.Printf("%s%s (%s...)\n", indents, ip, pubKey)
}

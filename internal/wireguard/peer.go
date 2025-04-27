package wireguard

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Invite struct {
	Interface DeviceConfig
	Server    PeerConfig
}

type PeerConfig struct {
	Name      string
	Cidr      *net.IPNet
	PublicKey PublicKey
}

func (c *PeerConfig) WriteInvite(
	w io.Writer,
	device *DeviceConfig,
) error {

	// TODO: write out the invite
	return nil
}

func PrintPeer(
	peer *wgtypes.Peer,
	indent int,
) {
	indents := strings.Repeat(" ", indent)
	ip := peer.AllowedIPs[0].IP
	pubKey := base64.RawStdEncoding.EncodeToString(peer.PublicKey[:])[:6]
	fmt.Printf("%s%s (%s...)\n", indents, ip, pubKey)
}

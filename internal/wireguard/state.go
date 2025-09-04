package wireguard

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Peer represents a single peer in a WireGuard configuration.
type Peer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []*net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
}

// Interface represents the complete configuration for a WireGuard network interface.
// It holds all the state needed to apply to the kernel and generate a native .conf file.
type Interface struct {
	Name       string // The interface name, e.g., "cord-prod"
	PrivateKey wgtypes.Key
	Address    net.IPNet
	ListenPort int
	Peers      []Peer

	backend Backend // Internal field for OS-specific implementation
}

// Backend defines the set of operations for a WireGuard implementation.
type Backend interface {
	Up(iface *Interface, configPath string) error
	Down(iface *Interface, delete bool) error
	Sync(iface *Interface) error
}

// ToWgConfig converts the Interface configuration to a wg-quick compatible .conf file format.
func (i *Interface) ToWgConfig() string {
	var config strings.Builder

	// [Interface] section
	config.WriteString("[Interface]\n")
	config.WriteString(fmt.Sprintf("PrivateKey = %s\n", i.PrivateKey.String()))
	config.WriteString(fmt.Sprintf("Address = %s\n", i.Address.String()))

	if i.ListenPort > 0 {
		config.WriteString(fmt.Sprintf("ListenPort = %d\n", i.ListenPort))
	}

	// Add peers
	for _, peer := range i.Peers {
		config.WriteString("\n[Peer]")
		config.WriteString(fmt.Sprintf("\nPublicKey = %s", peer.PublicKey.String()))

		// Format AllowedIPs
		if len(peer.AllowedIPs) > 0 {
			var allowedIPs []string
			for _, ip := range peer.AllowedIPs {
				allowedIPs = append(allowedIPs, ip.String())
			}
			config.WriteString(fmt.Sprintf("\nAllowedIPs = %s", strings.Join(allowedIPs, ", ")))
		}

		// Add endpoint if specified
		if peer.Endpoint != nil {
			config.WriteString(fmt.Sprintf("\nEndpoint = %s", peer.Endpoint.String()))
		}

		// Add persistent keepalive if specified
		if peer.PersistentKeepalive > 0 {
			config.WriteString(fmt.Sprintf("\nPersistentKeepalive = %d", int(peer.PersistentKeepalive.Seconds())))
		}
	}

	return config.String()
}

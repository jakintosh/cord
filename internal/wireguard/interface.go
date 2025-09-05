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
type Interface struct {
	Name       string
	PrivateKey wgtypes.Key
	Address    net.IPNet
	ListenPort int
	Peers      []Peer

	backend Backend // Internal field for OS-specific implementation
}

// Up creates the network device if it doesn't exist, configures it with the
// current state of the Interface object, and brings it up.
// It also writes the native .conf file to the specified path.
func (i *Interface) Up(configPath string) error {
	return i.backend.Up(i, configPath)
}

// Down brings the interface down and, optionally, deletes it.
func (i *Interface) Down(delete bool) error {
	return i.backend.Down(i, delete)
}

// Sync applies only the changes to the peer list to a live interface
// without tearing it down. This is more efficient for updates.
func (i *Interface) Sync() error {
	return i.backend.Sync(i)
}

// AddPeer adds a peer to the interface's configuration.
func (i *Interface) AddPeer(peer Peer) {
	i.Peers = append(i.Peers, peer)
}

// RemovePeer removes a peer from the configuration by its public key.
func (i *Interface) RemovePeer(publicKey wgtypes.Key) {
	for j, peer := range i.Peers {
		if peer.PublicKey == publicKey {
			// Remove the peer by slicing
			i.Peers = append(i.Peers[:j], i.Peers[j+1:]...)
			return
		}
	}
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

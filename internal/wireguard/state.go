package wireguard

import (
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Peer represents a single peer in a WireGuard configuration.
type Peer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
}

// Interface represents the complete configuration for a WireGuard network interface.
// It holds all the state needed to apply to the kernel and generate a native .conf file.
type Interface struct {
	Name       string      // The interface name, e.g., "cord-prod"
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
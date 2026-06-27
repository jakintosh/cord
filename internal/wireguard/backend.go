package wireguard

import (
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Backend defines the operations a WireGuard implementation must
// provide. This abstraction supports both kernel-based WireGuard
// (on Linux) and userspace implementations (on macOS and Linux).
type Backend interface {
	// Up creates the network device if it doesn't exist, configures
	// it with the given parameters, and brings it up.
	Up(cfg DeviceConfig) error

	// Down brings the device down and optionally deletes it.
	Down(name string) error

	// Delete removes the device from the system.
	Delete(name string) error

	// Status reports the live device state, including observed peer
	// endpoints and handshake times.
	Status(name string) (*DeviceStatus, error)

	// ApplyPeerOperations applies targeted peer changes without
	// disturbing peers absent from operations.
	ApplyPeerOperations(name string, operations []PeerOperation) error
}

// DeviceConfig holds the parameters needed to create and configure
// a WireGuard device.
type DeviceConfig struct {
	Name       string
	PrivateKey wgtypes.Key
	Address    net.IPNet
	ListenPort int
	MTU        int
	NoRoutes   bool
}

// DeviceStatus is the observed state of a live WireGuard device.
type DeviceStatus struct {
	Name       string
	ListenPort int
	Peers      []ObservedPeer
}

// ObservedPeer is the configuration and runtime state reported by
// WireGuard for a single peer.
type ObservedPeer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
	LastHandshake       time.Time
	ReceiveBytes        int64
	TransmitBytes       int64
}

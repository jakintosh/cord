package wireguard

import (
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// BackendType selects which WireGuard implementation drives an Interface.
type BackendType int

const (
	// BackendAuto picks the best implementation for the current OS:
	// kernel WireGuard on Linux, userspace (wireguard-go) on macOS.
	BackendAuto BackendType = iota
	BackendKernel
	BackendUserspace
)

// Backend defines the set of operations for a WireGuard implementation.
// This abstraction allows the package to support both kernel-based WireGuard
// (on Linux) and userspace implementations (on macOS, and optionally Linux).
type Backend interface {
	// Up creates the network device if it doesn't exist, configures it with the
	// current state of the Interface object, and brings it up.
	// It also writes the native .conf file to configPath when non-empty.
	Up(iface *Interface, configPath string) error

	// Down brings the interface down and, optionally, deletes it.
	Down(iface *Interface, delete bool) error

	// Status reports the live device state, including observed peer
	// endpoints and handshake times. Used for endpoint gossip.
	Status(iface *Interface) (*DeviceStatus, error)

	// ApplyPeerOperations applies targeted peer changes without disturbing
	// peers absent from operations.
	ApplyPeerOperations(iface *Interface, operations []PeerOperation) error
}

// ObservedPeer is the configuration and runtime state reported by WireGuard.
type ObservedPeer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
	LastHandshake       time.Time
	ReceiveBytes        int64
	TransmitBytes       int64
}

// PeerStatus is retained as the status-facing name used by client callbacks.
type PeerStatus = ObservedPeer

// DeviceStatus is the observed state of a live WireGuard device.
type DeviceStatus struct {
	Name       string
	ListenPort int
	Peers      []ObservedPeer
}

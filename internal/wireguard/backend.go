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

	// Sync applies only the changes to the peer list to a live interface
	// without tearing it down. This is more efficient for updates.
	Sync(iface *Interface) error

	// Status reports the live device state, including observed peer
	// endpoints and handshake times. Used for endpoint gossip.
	Status(iface *Interface) (*DeviceStatus, error)
}

// PeerStatus is the observed state of a single peer on a live device.
type PeerStatus struct {
	PublicKey     wgtypes.Key
	Endpoint      *net.UDPAddr
	LastHandshake time.Time
}

// DeviceStatus is the observed state of a live WireGuard device.
type DeviceStatus struct {
	Name       string
	ListenPort int
	Peers      []PeerStatus
}

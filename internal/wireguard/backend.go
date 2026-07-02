package wireguard

import (
	"net"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Backend defines the operations a WireGuard implementation must
// provide. This abstraction supports both kernel-based WireGuard
// (on Linux) and userspace implementations (on macOS and Linux).
type Backend interface {
	// Up creates the network device if it doesn't exist, configures
	// it with the given parameters, and brings it up.
	Up(name string, privateKey wgtypes.Key, address net.IPNet, listenPort int, mtu int, noRoutes bool) error

	// Down brings the device down and optionally deletes it.
	Down(name string) error

	// Delete removes the device from the system.
	Delete(name string) error

	// GetPeers returns the live peers observed on this device,
	// including their endpoints and last handshake times.
	GetPeers(name string) ([]Peer, error)

	// ModifyPeers applies targeted peer changes without
	// disturbing peers absent from operations.
	ModifyPeers(name string, operations []PeerOperation) error
}

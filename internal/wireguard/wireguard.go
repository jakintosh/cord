package wireguard

import (
	"fmt"
	"net"
	"runtime"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// NewInterface creates a new in-memory representation of a WireGuard interface.
// It will select the appropriate OS backend automatically.
func NewInterface(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	listenPort int,
) (*Interface, error) {
	iface := &Interface{
		Name:       name,
		PrivateKey: privateKey,
		Address:    address,
		ListenPort: listenPort,
		Peers:      make([]Peer, 0),
	}

	// Select the appropriate backend based on the OS
	switch runtime.GOOS {
	case "linux":
		iface.backend = &KernelBackend{}
	case "darwin", "windows":
		iface.backend = &UserspaceBackend{}
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return iface, nil
}

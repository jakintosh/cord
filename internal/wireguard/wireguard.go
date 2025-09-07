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

// Key generation utilities

// GeneratePrivateKey generates a new WireGuard private key.
func GeneratePrivateKey() (wgtypes.Key, error) {
	return wgtypes.GeneratePrivateKey()
}

// ParseKey parses a base64-encoded WireGuard key string.
func ParseKey(keyStr string) (wgtypes.Key, error) {
	key, err := wgtypes.ParseKey(keyStr)
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to parse key: %w", err)
	}
	return key, nil
}

// Legacy compatibility - temporary helper for network creation
// Ultimately, this should live somewhere else, this is a cord
// network config, not a wireguard device config (even though there
// is crossover)
type DeviceConfig struct {
	PrivateKey wgtypes.Key
	Cidr       *net.IPNet
	ListenPort uint16
}

func NewDeviceConfig(
	privateKey wgtypes.Key,
	networkCidr *net.IPNet,
	address net.IP,
	port uint16,
) (*DeviceConfig, error) {
	if !networkCidr.Contains(address) {
		return nil, fmt.Errorf(
			"address '%s' is not within cidr '%s'",
			address.String(), networkCidr.String(),
		)
	}
	return &DeviceConfig{
		PrivateKey: privateKey,
		Cidr: &net.IPNet{
			IP:   address,
			Mask: networkCidr.Mask,
		},
		ListenPort: port,
	}, nil
}

func (c *DeviceConfig) Write(w any) error {
	// This is a legacy compatibility shim
	// In the new architecture, server config should be handled differently
	// For now, just return nil to prevent build errors
	return nil
}

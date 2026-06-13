package wireguard

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const defaultMTU = 1420

const (
	maxInterfaceNameBytes = 15
	inviteInterfaceSuffix = "-i"
)

// Peer represents a single peer in a WireGuard configuration.
type Peer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
}

// Interface represents the complete configuration for a WireGuard network interface.
type Interface struct {
	Name       string
	PrivateKey wgtypes.Key
	Address    net.IPNet
	ListenPort int
	MTU        int
	NoRoutes   bool
	Peers      []Peer

	backend  Backend
	realName string // actual OS device name (e.g. utun4 on darwin)
}

// NewInterface creates a new in-memory representation of a WireGuard
// interface, backed by the requested implementation. On macOS the OS
// chooses the real device name (utunN); use DeviceName() after Up().
func NewInterface(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	listenPort int,
	backendType BackendType,
) (*Interface, error) {
	if err := validateInterfaceName(name); err != nil {
		return nil, err
	}

	backend, err := newBackend(backendType)
	if err != nil {
		return nil, err
	}

	return &Interface{
		Name:       name,
		PrivateKey: privateKey,
		Address:    address,
		ListenPort: listenPort,
		MTU:        defaultMTU,
		Peers:      make([]Peer, 0),
		backend:    backend,
	}, nil
}

// NetworkInterfaceNames returns the OS interface names for a cord network.
// Linux limits interface names to 15 bytes, including role suffixes.
func NetworkInterfaceNames(network string) (string, string, error) {
	if strings.HasSuffix(network, inviteInterfaceSuffix) {
		return "", "", fmt.Errorf(
			"network name '%s' must not end with reserved suffix '%s'",
			network,
			inviteInterfaceSuffix,
		)
	}

	main := network
	invite := network + inviteInterfaceSuffix
	if err := validateInterfaceName(main); err != nil {
		return "", "", fmt.Errorf("invalid main interface name: %w", err)
	}
	if err := validateInterfaceName(invite); err != nil {
		return "", "", fmt.Errorf(
			"network name '%s' is too long: invite interface name '%s' exceeds %d bytes",
			network,
			invite,
			maxInterfaceNameBytes,
		)
	}
	return main, invite, nil
}

func validateInterfaceName(name string) error {
	if name == "" {
		return fmt.Errorf("interface name must not be empty")
	}
	if len(name) > maxInterfaceNameBytes {
		return fmt.Errorf(
			"interface name '%s' exceeds %d bytes",
			name,
			maxInterfaceNameBytes,
		)
	}
	return nil
}

// DeviceName returns the actual OS device name once the interface is up.
// Before Up() (and everywhere on Linux) it is the configured name.
func (i *Interface) DeviceName() string {
	if i.realName != "" {
		return i.realName
	}
	return i.Name
}

// Up creates the network device if it doesn't exist, configures it with the
// current state of the Interface object, and brings it up.
// It also writes the native .conf file to configPath when non-empty.
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

// Status reports the live device state (peer endpoints, handshakes).
func (i *Interface) Status() (*DeviceStatus, error) {
	return i.backend.Status(i)
}

// WaitForHandshake waits until the live device reports a completed
// handshake with the requested peer.
func (i *Interface) WaitForHandshake(
	publicKey wgtypes.Key,
	timeout time.Duration,
	onStatus func(PeerStatus),
) error {
	deadline := time.Now().Add(timeout)
	var lastStatusErr error

	for {
		status, err := i.Status()
		if err != nil {
			lastStatusErr = err
		} else {
			lastStatusErr = nil
			for _, peer := range status.Peers {
				if peer.PublicKey == publicKey {
					if onStatus != nil {
						onStatus(peer)
					}
					if !peer.LastHandshake.IsZero() {
						return nil
					}
				}
			}
		}

		if time.Now().After(deadline) {
			if lastStatusErr != nil {
				return fmt.Errorf("failed to inspect WireGuard status: %w", lastStatusErr)
			}
			return fmt.Errorf("no handshake completed within %s", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// AddPeer adds a peer to the interface's configuration.
func (i *Interface) AddPeer(peer Peer) {
	i.Peers = append(i.Peers, peer)
}

// RemovePeer removes a peer from the configuration by its public key.
func (i *Interface) RemovePeer(publicKey wgtypes.Key) {
	for j, peer := range i.Peers {
		if peer.PublicKey == publicKey {
			i.Peers = append(i.Peers[:j], i.Peers[j+1:]...)
			return
		}
	}
}

// SetPeers replaces the entire peer list.
func (i *Interface) SetPeers(peers []Peer) {
	i.Peers = peers
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

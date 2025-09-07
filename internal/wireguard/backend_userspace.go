package wireguard

import (
	"fmt"
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	defaultMTU = 1420
)

// UserspaceBackend implements the Backend interface for non-Linux systems
// (macOS, Windows) and optionally for Linux. This implementation runs the
// WireGuard protocol in userspace.
type UserspaceBackend struct {
	device    *device.Device
	tunDevice tun.Device
	stopChan  chan struct{}
	isRunning bool
	mu        sync.Mutex
}

func (b *UserspaceBackend) Up(
	iface *Interface,
	configPath string,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Idempotency check: if already running, return nil
	if b.isRunning {
		return nil
	}

	// Create TUN device
	tunDevice, err := tun.CreateTUN(iface.Name, defaultMTU)
	if err != nil {
		return fmt.Errorf("failed to create TUN device: %w", err)
	}

	// Configure TUN device (bring up with IP address)
	if err := configureTunOS(iface.Name, iface.Address, defaultMTU); err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to configure TUN device: %w", err)
	}

	// Start wireguard-go device
	device, err := b.startDevice(tunDevice)
	if err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to start WireGuard device: %w", err)
	}

	// Apply initial configuration
	if err := b.applyConfig(device, iface.PrivateKey, iface.ListenPort, iface.Peers); err != nil {
		device.Close()
		tunDevice.Close()
		return fmt.Errorf("failed to apply initial configuration: %w", err)
	}

	// Write native config file
	if err := os.WriteFile(configPath, []byte(iface.ToWgConfig()), 0600); err != nil {
		device.Close()
		tunDevice.Close()
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update backend state
	b.device = device
	b.tunDevice = tunDevice
	b.stopChan = make(chan struct{})
	b.isRunning = true

	// Start background goroutine to handle cleanup
	go b.waitForStop()

	return nil
}

func (b *UserspaceBackend) Down(
	iface *Interface,
	delete bool,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Idempotency check: if not running, return nil
	if !b.isRunning {
		return nil
	}

	// Signal shutdown to the background goroutine
	close(b.stopChan)

	return nil
}

func (b *UserspaceBackend) Sync(
	iface *Interface,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// State check: ensure the backend is running
	if !b.isRunning || b.device == nil {
		return fmt.Errorf("cannot sync: WireGuard device is not running")
	}

	// Apply the updated configuration to the running device
	if err := b.applyConfig(b.device, iface.PrivateKey, iface.ListenPort, iface.Peers); err != nil {
		return fmt.Errorf("failed to sync configuration: %w", err)
	}

	return nil
}

// startDevice instantiates a new device.Device from wireguard-go, connects it to the TUN interface, and sets it up
func (b *UserspaceBackend) startDevice(
	tunDevice tun.Device,
) (
	*device.Device,
	error,
) {
	logger := device.NewLogger(device.LogLevelError, "")
	device := device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)

	// Wait for the device to be ready
	device.Up()

	return device, nil
}

// applyConfig constructs the configuration in the line-based uapi format and sends it to the running device.Device via its IpcSet method
func (b *UserspaceBackend) applyConfig(
	device *device.Device,
	key wgtypes.Key,
	port int,
	peers []Peer,
) error {
	config := fmt.Sprintf("private_key=%s\n", key.String())

	if port > 0 {
		config += fmt.Sprintf("listen_port=%d\n", port)
	}

	// Clear existing peers first
	config += "replace_peers=true\n"

	// Add each peer
	for _, peer := range peers {
		config += fmt.Sprintf("public_key=%s\n", peer.PublicKey.String())

		// Add allowed IPs
		for _, allowedIP := range peer.AllowedIPs {
			config += fmt.Sprintf("allowed_ip=%s\n", allowedIP.String())
		}

		// Add endpoint if specified
		if peer.Endpoint != nil {
			config += fmt.Sprintf("endpoint=%s\n", peer.Endpoint.String())
		}

		// Add persistent keepalive if specified
		if peer.PersistentKeepalive > 0 {
			config += fmt.Sprintf("persistent_keepalive_interval=%d\n", int(peer.PersistentKeepalive.Seconds()))
		}
	}

	return device.IpcSet(config)
}

// waitForStop runs in a background goroutine and handles graceful shutdown
func (b *UserspaceBackend) waitForStop() {
	<-b.stopChan

	// Clean up resources
	if b.device != nil {
		b.device.Close()
	}
	if b.tunDevice != nil {
		b.tunDevice.Close()
	}

	// Update backend state
	b.device = nil
	b.tunDevice = nil
	b.stopChan = nil
	b.isRunning = false
}

package wireguard

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// UserspaceBackend implements the Backend interface using wireguard-go.
// It is the only option on macOS and an opt-in alternative on Linux.
// The device lives inside this process: when the process exits, the
// interface disappears with it.
type UserspaceBackend struct {
	device    *device.Device
	tunDevice tun.Device
	mu        sync.Mutex
}

func (b *UserspaceBackend) Up(
	iface *Interface,
	configPath string,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Idempotency check: if already running, just re-apply config
	if b.device != nil {
		return b.applyConfig(iface)
	}

	mtu := iface.MTU
	if mtu <= 0 {
		mtu = defaultMTU
	}

	// Create TUN device (on macOS the OS assigns a utunN name)
	tunDevice, err := tun.CreateTUN(tunName(iface.Name), mtu)
	if err != nil {
		return fmt.Errorf("failed to create TUN device: %w", err)
	}

	realName, err := tunDevice.Name()
	if err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to get TUN device name: %w", err)
	}
	iface.realName = realName

	// Assign address, set MTU, bring the OS device up
	if err := configureTunOS(realName, iface.Address, mtu, iface.NoRoutes); err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to configure TUN device: %w", err)
	}

	// Start the wireguard-go device
	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", realName))
	dev := device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)

	b.device = dev
	b.tunDevice = tunDevice

	// Apply configuration (private key, port, peers)
	if err := b.applyConfig(iface); err != nil {
		b.closeLocked()
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	if err := dev.Up(); err != nil {
		b.closeLocked()
		return fmt.Errorf("failed to bring device up: %w", err)
	}

	// Write the native config file
	if configPath != "" {
		if err := os.WriteFile(configPath, []byte(iface.ToWgConfig()), 0600); err != nil {
			b.closeLocked()
			return fmt.Errorf("failed to write config file: %w", err)
		}
	}

	return nil
}

func (b *UserspaceBackend) Down(
	iface *Interface,
	delete bool,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Idempotency check: if not running, return nil
	if b.device == nil {
		return nil
	}

	b.closeLocked()
	iface.realName = ""
	return nil
}

func (b *UserspaceBackend) Sync(
	iface *Interface,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.device == nil {
		return fmt.Errorf("cannot sync: WireGuard device is not running")
	}

	if err := b.applyConfig(iface); err != nil {
		return fmt.Errorf("failed to sync configuration: %w", err)
	}

	return nil
}

func (b *UserspaceBackend) Status(
	iface *Interface,
) (
	*DeviceStatus,
	error,
) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.device == nil {
		return nil, fmt.Errorf("WireGuard device is not running")
	}

	raw, err := b.device.IpcGet()
	if err != nil {
		return nil, fmt.Errorf("failed to query device: %w", err)
	}

	return parseUapiStatus(iface.DeviceName(), raw)
}

// closeLocked tears down the device; callers must hold b.mu.
func (b *UserspaceBackend) closeLocked() {
	if b.device != nil {
		// closes the TUN device as well
		b.device.Close()
	}
	b.device = nil
	b.tunDevice = nil
}

// applyConfig constructs the configuration in the line-based uapi format
// and sends it to the running device via IpcSet. Callers must hold b.mu.
func (b *UserspaceBackend) applyConfig(iface *Interface) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "private_key=%s\n", hex.EncodeToString(iface.PrivateKey[:]))

	if iface.ListenPort > 0 {
		fmt.Fprintf(&sb, "listen_port=%d\n", iface.ListenPort)
	}

	// Clear existing peers first
	sb.WriteString("replace_peers=true\n")

	for _, peer := range iface.Peers {
		fmt.Fprintf(&sb, "public_key=%s\n", hex.EncodeToString(peer.PublicKey[:]))

		for _, allowedIP := range peer.AllowedIPs {
			fmt.Fprintf(&sb, "allowed_ip=%s\n", allowedIP.String())
		}

		if peer.Endpoint != nil {
			fmt.Fprintf(&sb, "endpoint=%s\n", peer.Endpoint.String())
		}

		if peer.PersistentKeepalive > 0 {
			fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", int(peer.PersistentKeepalive.Seconds()))
		}
	}

	return b.device.IpcSet(sb.String())
}

// parseUapiStatus converts wireguard-go's uapi "get" output into a DeviceStatus.
func parseUapiStatus(name string, raw string) (*DeviceStatus, error) {
	status := &DeviceStatus{Name: name}

	var peer *PeerStatus
	var handshakeSec int64

	flushPeer := func() {
		if peer != nil {
			if handshakeSec > 0 {
				peer.LastHandshake = time.Unix(handshakeSec, 0)
			}
			status.Peers = append(status.Peers, *peer)
		}
		peer = nil
		handshakeSec = 0
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		switch key {
		case "listen_port":
			port, err := strconv.Atoi(value)
			if err == nil {
				status.ListenPort = port
			}
		case "public_key":
			flushPeer()
			keyBytes, err := hex.DecodeString(value)
			if err != nil || len(keyBytes) != wgtypes.KeyLen {
				return nil, fmt.Errorf("invalid public key in device status: %s", value)
			}
			peer = &PeerStatus{PublicKey: wgtypes.Key(keyBytes)}
		case "endpoint":
			if peer != nil {
				if addr, err := net.ResolveUDPAddr("udp", value); err == nil {
					peer.Endpoint = addr
				}
			}
		case "last_handshake_time_sec":
			if sec, err := strconv.ParseInt(value, 10, 64); err == nil {
				handshakeSec = sec
			}
		}
	}
	flushPeer()

	return status, nil
}

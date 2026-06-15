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
		return b.applyDeviceConfig(iface)
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

	// Apply device configuration; peers are reconciled after the device is up.
	if err := b.applyDeviceConfig(iface); err != nil {
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

func (b *UserspaceBackend) ApplyPeerOperations(
	iface *Interface,
	operations []PeerOperation,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.device == nil {
		return fmt.Errorf("cannot sync: WireGuard device is not running")
	}

	if err := b.applyPeerOperations(operations); err != nil {
		return fmt.Errorf("failed to apply peer operations: %w", err)
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

// applyDeviceConfig applies only device-level configuration. Callers must hold b.mu.
func (b *UserspaceBackend) applyDeviceConfig(iface *Interface) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "private_key=%s\n", hex.EncodeToString(iface.PrivateKey[:]))

	if iface.ListenPort > 0 {
		fmt.Fprintf(&sb, "listen_port=%d\n", iface.ListenPort)
	}

	return b.device.IpcSet(sb.String())
}

// applyPeerOperations translates standardized operations into WireGuard UAPI.
// Callers must hold b.mu.
func (b *UserspaceBackend) applyPeerOperations(operations []PeerOperation) error {
	return b.device.IpcSet(peerOperationsUAPI(operations))
}

func peerOperationsUAPI(operations []PeerOperation) string {
	var sb strings.Builder
	for _, operation := range operations {
		peer := operation.Peer
		fmt.Fprintf(&sb, "public_key=%s\n", hex.EncodeToString(peer.PublicKey[:]))
		switch operation.Type {
		case PeerRemove:
			sb.WriteString("remove=true\n")
		case PeerAdd:
			sb.WriteString("replace_allowed_ips=true\n")
			writeAllowedIPs(&sb, peer.AllowedIPs)
			if peer.EndpointPolicy != EndpointDynamic && peer.Endpoint != nil {
				fmt.Fprintf(&sb, "endpoint=%s\n", peer.Endpoint.String())
			}
			fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", int(peer.PersistentKeepalive.Seconds()))
		case PeerUpdate:
			sb.WriteString("update_only=true\n")
			if operation.UpdateAllowedIPs {
				sb.WriteString("replace_allowed_ips=true\n")
				writeAllowedIPs(&sb, peer.AllowedIPs)
			}
			if operation.UpdateEndpoint && peer.Endpoint != nil {
				fmt.Fprintf(&sb, "endpoint=%s\n", peer.Endpoint.String())
			}
			if operation.UpdateKeepalive {
				fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", int(peer.PersistentKeepalive.Seconds()))
			}
		}
	}
	return sb.String()
}

func writeAllowedIPs(sb *strings.Builder, allowedIPs []net.IPNet) {
	for _, allowedIP := range allowedIPs {
		fmt.Fprintf(sb, "allowed_ip=%s\n", allowedIP.String())
	}
}

// parseUapiStatus converts wireguard-go's uapi "get" output into a DeviceStatus.
func parseUapiStatus(name string, raw string) (*DeviceStatus, error) {
	status := &DeviceStatus{Name: name}

	var peer *PeerStatus
	var handshakeSec int64
	var handshakeNsec int64

	flushPeer := func() {
		if peer != nil {
			if handshakeSec > 0 {
				peer.LastHandshake = time.Unix(handshakeSec, handshakeNsec)
			}
			status.Peers = append(status.Peers, *peer)
		}
		peer = nil
		handshakeSec = 0
		handshakeNsec = 0
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
		case "last_handshake_time_nsec":
			if nsec, err := strconv.ParseInt(value, 10, 64); err == nil {
				handshakeNsec = nsec
			}
		case "persistent_keepalive_interval":
			if peer != nil {
				if seconds, err := strconv.Atoi(value); err == nil {
					peer.PersistentKeepalive = time.Duration(seconds) * time.Second
				}
			}
		case "allowed_ip":
			if peer != nil {
				if _, allowed, err := net.ParseCIDR(value); err == nil {
					peer.AllowedIPs = append(peer.AllowedIPs, *allowed)
				}
			}
		case "rx_bytes":
			if peer != nil {
				peer.ReceiveBytes, _ = strconv.ParseInt(value, 10, 64)
			}
		case "tx_bytes":
			if peer != nil {
				peer.TransmitBytes, _ = strconv.ParseInt(value, 10, 64)
			}
		}
	}
	flushPeer()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan device status: %w", err)
	}

	return status, nil
}

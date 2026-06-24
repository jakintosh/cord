package wireguard

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// UserspaceBackend implements Backend using wireguard-go. The device
// lives inside this process.
type UserspaceBackend struct {
	wgDevice  *device.Device
	tunDevice tun.Device
	mu        sync.Mutex
}

func (b *UserspaceBackend) Up(cfg DeviceConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.wgDevice != nil {
		return b.applyDeviceConfig(cfg)
	}

	mtu := cfg.MTU
	if mtu <= 0 {
		mtu = defaultMTU
	}

	tunName := tunName(cfg.Name)
	tunDev, err := tun.CreateTUN(tunName, mtu)
	if err != nil {
		return fmt.Errorf("wireguard: create tun: %w", err)
	}

	realName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return fmt.Errorf("wireguard: get tun name: %w", err)
	}

	if err := configureTunOS(realName, cfg.Address, mtu, cfg.NoRoutes); err != nil {
		tunDev.Close()
		return fmt.Errorf("wireguard: configure tun: %w", err)
	}

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", realName))
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	b.wgDevice = dev
	b.tunDevice = tunDev

	if err := b.applyDeviceConfig(cfg); err != nil {
		b.closeLocked()
		return fmt.Errorf("wireguard: apply device config: %w", err)
	}

	if err := dev.Up(); err != nil {
		b.closeLocked()
		return fmt.Errorf("wireguard: bring device up: %w", err)
	}

	return nil
}

func (b *UserspaceBackend) Down(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.wgDevice == nil {
		return nil
	}

	b.closeLocked()
	return nil
}

func (b *UserspaceBackend) Delete(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.wgDevice == nil {
		return nil
	}

	b.closeLocked()
	return nil
}

func (b *UserspaceBackend) Status(name string) (*DeviceStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.wgDevice == nil {
		return nil, fmt.Errorf("wireguard: device not running")
	}

	raw, err := b.wgDevice.IpcGet()
	if err != nil {
		return nil, fmt.Errorf("wireguard: ipc get: %w", err)
	}

	return parseUAPIStatus(name, raw)
}

func (b *UserspaceBackend) ApplyPeerOperations(name string, operations []PeerOperation) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.wgDevice == nil {
		return fmt.Errorf("wireguard: device not running")
	}

	return b.wgDevice.IpcSet(peerOperationsUAPI(operations))
}

func (b *UserspaceBackend) closeLocked() {
	if b.wgDevice != nil {
		b.wgDevice.Close()
	}
	b.wgDevice = nil
	b.tunDevice = nil
}

func (b *UserspaceBackend) applyDeviceConfig(cfg DeviceConfig) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "private_key=%s\n", hex.EncodeToString(cfg.PrivateKey[:]))

	if cfg.ListenPort > 0 {
		fmt.Fprintf(&sb, "listen_port=%d\n", cfg.ListenPort)
	}

	return b.wgDevice.IpcSet(sb.String())
}

func peerOperationsUAPI(operations []PeerOperation) string {
	var sb strings.Builder
	for _, op := range operations {
		peer := op.Peer
		fmt.Fprintf(&sb, "public_key=%s\n", hex.EncodeToString(peer.PublicKey[:]))
		switch op.Type {
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
			if op.UpdateAllowedIPs {
				sb.WriteString("replace_allowed_ips=true\n")
				writeAllowedIPs(&sb, peer.AllowedIPs)
			}
			if op.UpdateEndpoint && peer.Endpoint != nil {
				fmt.Fprintf(&sb, "endpoint=%s\n", peer.Endpoint.String())
			}
			if op.UpdateKeepalive {
				fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", int(peer.PersistentKeepalive.Seconds()))
			}
		}
	}
	return sb.String()
}

func writeAllowedIPs(sb *strings.Builder, allowedIPs []net.IPNet) {
	for _, ip := range allowedIPs {
		fmt.Fprintf(sb, "allowed_ip=%s\n", ip.String())
	}
}

func parseUAPIStatus(name string, raw string) (*DeviceStatus, error) {
	status := &DeviceStatus{Name: name}

	var peer *ObservedPeer
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
				return nil, fmt.Errorf("wireguard: invalid public key in status: %s", value)
			}
			peer = &ObservedPeer{PublicKey: wgtypes.Key(keyBytes)}
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
		return nil, fmt.Errorf("wireguard: scan status: %w", err)
	}

	return status, nil
}

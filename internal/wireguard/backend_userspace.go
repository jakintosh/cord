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

// userspaceDevice bundles a wireguard-go Device with its TUN.
type userspaceDevice struct {
	wg  *device.Device
	tun tun.Device
}

// UserspaceBackend implements Backend using wireguard-go. Each device
// lives inside this process, keyed by name.
type UserspaceBackend struct {
	devices map[string]*userspaceDevice
	mu      sync.Mutex
}

func (b *UserspaceBackend) Up(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	listenPort int,
	mtu int,
	noRoutes bool,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.devices == nil {
		b.devices = make(map[string]*userspaceDevice)
	}

	if existing, ok := b.devices[name]; ok {
		return b.applyDeviceConfig(existing.wg, privateKey, listenPort)
	}

	if mtu <= 0 {
		mtu = defaultMTU
	}

	tunDev, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return fmt.Errorf("wireguard: create tun: %w", err)
	}

	realName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return fmt.Errorf("wireguard: get tun name: %w", err)
	}

	if err := configureTunOS(realName, address, mtu, noRoutes); err != nil {
		tunDev.Close()
		return fmt.Errorf("wireguard: configure tun: %w", err)
	}

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", realName))
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	if err := b.applyDeviceConfig(dev, privateKey, listenPort); err != nil {
		dev.Close()
		tunDev.Close()
		return fmt.Errorf("wireguard: apply device config: %w", err)
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		tunDev.Close()
		return fmt.Errorf("wireguard: bring device up: %w", err)
	}

	b.devices[name] = &userspaceDevice{
		wg:  dev,
		tun: tunDev,
	}
	return nil
}

func (b *UserspaceBackend) Down(
	name string,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.devices[name]
	if !ok {
		return nil
	}

	d.wg.Close()
	delete(b.devices, name)
	return nil
}

func (b *UserspaceBackend) Delete(
	name string,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.devices[name]
	if !ok {
		return nil
	}

	d.wg.Close()
	delete(b.devices, name)
	return nil
}

func (b *UserspaceBackend) GetPeers(
	name string,
) (
	[]Peer,
	error,
) {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.devices[name]
	if !ok {
		return nil, fmt.Errorf("wireguard: device not running")
	}

	raw, err := d.wg.IpcGet()
	if err != nil {
		return nil, fmt.Errorf("wireguard: ipc get: %w", err)
	}

	return parseUAPIPeers(raw)
}

func (b *UserspaceBackend) ModifyPeers(
	name string,
	operations []PeerOperation,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.devices[name]
	if !ok {
		return fmt.Errorf("wireguard: device not running")
	}

	return d.wg.IpcSet(peerOperationsUAPI(operations))
}

func (b *UserspaceBackend) applyDeviceConfig(
	dev *device.Device,
	privateKey wgtypes.Key,
	listenPort int,
) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "private_key=%s\n", hex.EncodeToString(privateKey[:]))

	if listenPort > 0 {
		fmt.Fprintf(&sb, "listen_port=%d\n", listenPort)
	}

	return dev.IpcSet(sb.String())
}

func peerOperationsUAPI(
	operations []PeerOperation,
) string {
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

func writeAllowedIPs(
	sb *strings.Builder,
	allowedIPs []net.IPNet,
) {
	for _, ip := range allowedIPs {
		fmt.Fprintf(sb, "allowed_ip=%s\n", ip.String())
	}
}

func parseUAPIPeers(
	raw string,
) (
	[]Peer,
	error,
) {
	var peers []Peer

	var peer *Peer
	var handshakeSec int64
	var handshakeNsec int64

	flushPeer := func() {
		if peer != nil {
			if handshakeSec > 0 {
				peer.LastHandshake = time.Unix(handshakeSec, handshakeNsec)
			}
			peers = append(peers, *peer)
		}
		peer = nil
		handshakeSec = 0
		handshakeNsec = 0
	}

	reader := strings.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		switch key {
		case "listen_port":
			continue

		case "public_key":
			flushPeer()
			keyBytes, err := hex.DecodeString(value)
			if err != nil || len(keyBytes) != wgtypes.KeyLen {
				return nil, fmt.Errorf("wireguard: invalid public key in status: %s", value)
			}
			peer = &Peer{
				PublicKey: wgtypes.Key(keyBytes),
			}

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

	return peers, nil
}

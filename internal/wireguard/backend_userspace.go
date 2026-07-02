package wireguard

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// UserspaceBackend implements Backend using wireguard-go.
type UserspaceBackend struct{}

func (b *UserspaceBackend) CreateDevice(
	cfg DeviceConfig,
) (
	BackendDevice,
	error,
) {
	mtu := cfg.MTU
	if mtu <= 0 {
		mtu = defaultMTU
	}

	tunDev, err := tun.CreateTUN(cfg.Name, mtu)
	if err != nil {
		return nil, fmt.Errorf("wireguard: create tun: %w", err)
	}

	realName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return nil, fmt.Errorf("wireguard: get tun name: %w", err)
	}

	if err := configureTunOS(realName, cfg.Address, mtu); err != nil {
		tunDev.Close()
		return nil, fmt.Errorf("wireguard: configure tun: %w", err)
	}

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", realName))
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	privKey, err := parseKey(cfg.PrivateKey)
	if err != nil {
		dev.Close()
		tunDev.Close()
		return nil, fmt.Errorf("wireguard: parse key: %w", err)
	}

	if err := applyDeviceConfig(dev, privKey, int(cfg.ListenPort)); err != nil {
		dev.Close()
		tunDev.Close()
		return nil, fmt.Errorf("wireguard: apply device config: %w", err)
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		tunDev.Close()
		return nil, fmt.Errorf("wireguard: bring device up: %w", err)
	}

	return &userspaceDeviceHandle{
		name: realName,
		wg:   dev,
		tun:  tunDev,
	}, nil
}

// userspaceDeviceHandle is a BackendDevice backed by wireguard-go.
type userspaceDeviceHandle struct {
	name string
	wg   *device.Device
	tun  tun.Device
}

func (h *userspaceDeviceHandle) Peers() (
	[]PeerStatus,
	error,
) {
	raw, err := h.wg.IpcGet()
	if err != nil {
		return nil, fmt.Errorf("wireguard: ipc get: %w", err)
	}
	return parseUAPIPeers(raw)
}

func (h *userspaceDeviceHandle) ApplyPeers(
	ops []PeerOp,
) error {
	return h.wg.IpcSet(peerOperationsUAPI(ops))
}

func (h *userspaceDeviceHandle) Close() error {
	h.wg.Close()
	h.tun.Close()
	return nil
}

func applyDeviceConfig(
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
	operations []PeerOp,
) string {
	var sb strings.Builder
	for _, op := range operations {
		key := op.Config.PublicKey
		fmt.Fprintf(&sb, "public_key=%s\n", hex.EncodeToString(key[:]))
		if op.Remove {
			sb.WriteString("remove=true\n")
			continue
		}
		sb.WriteString("replace_allowed_ips=true\n")
		writeAllowedIPs(&sb, op.Config.AllowedIPs)
		if op.Config.Endpoint != nil {
			fmt.Fprintf(&sb, "endpoint=%s\n", op.Config.Endpoint.String())
		}
		fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", int(op.Config.PersistentKeepalive.Seconds()))
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
	[]PeerStatus,
	error,
) {
	var peers []PeerStatus

	var peer *PeerStatus
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
			peer = &PeerStatus{
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

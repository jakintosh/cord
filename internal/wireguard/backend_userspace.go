package wireguard

import (
	"fmt"
	"net"
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
	WgDevice,
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

// userspaceDeviceHandle is a WgDevice backed by wireguard-go.
type userspaceDeviceHandle struct {
	name string
	wg   *device.Device
	tun  tun.Device
}

func (d *userspaceDeviceHandle) Peers() (
	[]PeerStatus,
	error,
) {
	raw, err := d.wg.IpcGet()
	if err != nil {
		return nil, fmt.Errorf("wireguard: ipc get: %w", err)
	}

	return parsePeersUAPI(raw)
}

func (d *userspaceDeviceHandle) ApplyPeers(
	ops []PeerOp,
) error {
	return d.wg.IpcSet(buildOpsUAPI(ops))
}

func (d *userspaceDeviceHandle) Close() error {
	d.wg.Close()
	d.tun.Close()
	return nil
}

func applyDeviceConfig(
	dev *device.Device,
	privateKey wgtypes.Key,
	listenPort int,
) error {
	var w uapiWriter
	w.SetHex("private_key", privateKey[:])
	if listenPort > 0 {
		w.SetInt("listen_port", listenPort)
	}
	return dev.IpcSet(w.String())
}

func buildOpsUAPI(
	operations []PeerOp,
) string {
	var w uapiWriter
	for _, op := range operations {
		w.SetHex("public_key", op.Config.PublicKey[:])
		if op.Remove {
			w.Set("remove", "true")
			continue
		}
		w.Set("replace_allowed_ips", "true")
		for _, ip := range op.Config.AllowedIPs {
			w.Set("allowed_ip", ip.String())
		}
		if op.Config.Endpoint != nil {
			w.Set("endpoint", op.Config.Endpoint.String())
		}
		w.SetInt("persistent_keepalive_interval", int(op.Config.PersistentKeepalive.Seconds()))
	}
	return w.String()
}

func parsePeersUAPI(
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

	p := newUAPIParser(raw)
	for p.Next() {
		switch p.Key() {
		case "listen_port":
			continue

		case "public_key":
			flushPeer()
			var keyBytes []byte
			if err := p.DecodeHex(&keyBytes); err != nil || len(keyBytes) != wgtypes.KeyLen {
				return nil, fmt.Errorf("wireguard: invalid public key in status: %s", p.Value())
			}
			peer = &PeerStatus{
				PublicKey: wgtypes.Key(keyBytes),
			}

		case "endpoint":
			if peer != nil {
				_ = p.DecodeUDPAddr(&peer.Endpoint)
			}

		case "last_handshake_time_sec":
			if peer != nil {
				_ = p.DecodeInt64(&handshakeSec)
			}

		case "last_handshake_time_nsec":
			if peer != nil {
				_ = p.DecodeInt64(&handshakeNsec)
			}

		case "persistent_keepalive_interval":
			if peer != nil {
				var seconds int
				if err := p.DecodeInt(&seconds); err == nil {
					peer.PersistentKeepalive = time.Duration(seconds) * time.Second
				}
			}

		case "allowed_ip":
			if peer != nil {
				var ipNet net.IPNet
				if err := p.DecodeCIDR(&ipNet); err == nil {
					peer.AllowedIPs = append(peer.AllowedIPs, ipNet)
				}
			}

		case "rx_bytes":
			if peer != nil {
				_ = p.DecodeInt64(&peer.ReceiveBytes)
			}

		case "tx_bytes":
			if peer != nil {
				_ = p.DecodeInt64(&peer.TransmitBytes)
			}
		}
	}

	flushPeer()
	if err := p.Err(); err != nil {
		return nil, fmt.Errorf("wireguard: scan status: %w", err)
	}

	return peers, nil
}

package wireguard

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

// UserspaceBackend implements Backend using wireguard-go.
type UserspaceBackend struct {
	log *slog.Logger
}

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

	if err := configureTunOS(realName, cfg.Route, mtu); err != nil {
		tunDev.Close()
		return nil, fmt.Errorf("wireguard: configure tun: %w", err)
	}

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), b.deviceLogger(realName))

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

// deviceLogger bridges wireguard-go's printf-style logger to slog.
// Verbose lines (handshake initiations/responses, keepalives) forward
// at Debug and are dropped entirely when debug is off, so the Sprintf
// cost is only paid when someone is watching.
func (b *UserspaceBackend) deviceLogger(
	name string,
) *device.Logger {
	base := b.log
	if base == nil {
		base = logging.Discard()
	}
	log := base.With("device", name)

	verbosef := device.DiscardLogf
	if log.Enabled(context.Background(), slog.LevelDebug) {
		verbosef = func(format string, args ...any) {
			log.Debug(fmt.Sprintf(format, args...))
		}
	}

	return &device.Logger{
		Verbosef: verbosef,
		Errorf: func(format string, args ...any) {
			log.Error(fmt.Sprintf(format, args...))
		},
	}
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
		w.SetHex("public_key", op.Target.PublicKey[:])
		if op.Remove {
			w.Set("remove", "true")
			continue
		}
		w.Set("replace_allowed_ips", "true")
		for _, ip := range op.Target.AllowedIPs {
			w.Set("allowed_ip", ip.String())
		}
		if op.Target.Endpoint != nil {
			w.Set("endpoint", op.Target.Endpoint.String())
		}
		w.SetInt("persistent_keepalive_interval", int(op.Target.PersistentKeepalive.Seconds()))
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

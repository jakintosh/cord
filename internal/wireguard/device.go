package wireguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"sync"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// DeviceConfig configures a persistent WireGuard device.
type DeviceConfig struct {
	Name        string
	PrivateKey  string
	Route       net.IPNet
	NetworkCIDR net.IPNet
	ListenPort  uint16
	MTU         int // 0 uses the default
}

// Device owns a configured WireGuard interface across starts and failures.
// All methods serialize access with a mutex held across backend calls.
type Device struct {
	config  DeviceConfig
	factory Backend
	log     *slog.Logger

	mu      sync.Mutex
	backend WgDevice
	desired map[wgtypes.Key]Peer
	lastErr error
}

// newDevice retains configuration without creating an interface.
func newDevice(
	config DeviceConfig,
	factory Backend,
	log *slog.Logger,
) *Device {
	config.Route.IP = slices.Clone(config.Route.IP)
	config.Route.Mask = slices.Clone(config.Route.Mask)
	config.NetworkCIDR.IP = slices.Clone(config.NetworkCIDR.IP)
	config.NetworkCIDR.Mask = slices.Clone(config.NetworkCIDR.Mask)

	return &Device{
		config:  config,
		factory: factory,
		log:     log.With("device", config.Name),
	}
}

// Name returns the current backend's OS interface name. Callers that require a
// working interface must reconcile first; a retained failed handle still has a
// name. A stopped wrapper never substitutes its configured logical name.
func (d *Device) Name() (
	string,
	error,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.backend == nil {
		return "", fmt.Errorf("wireguard: device is stopped")
	}
	return d.backend.Name(), nil
}

// Status reads interface availability without repairing it. Peer handshakes
// describe remote connectivity and do not determine device health.
func (d *Device) Status() resource.Status {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.statusLocked()
}

// Reconcile ensures the interface is running and restores desired peers after
// recreation. If the interface is already running, it returns without syncing
// peers. Backend calls are synchronous; cancellation is checked between them.
func (d *Device) Reconcile(
	ctx context.Context,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	status := d.statusLocked()
	switch status.State {
	case resource.Running:
		return nil

	case resource.Failed:
		d.lastErr = status.Error
		if err := d.closeLocked(); err != nil {
			return err
		}

	case resource.Stopped:
		// Continue below to start the device.
		break

	default:
		return fmt.Errorf("wireguard: unexpected device state %q", status.State)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return d.startLocked(ctx)
}

// Close releases the underlying interface, retaining configuration and desired
// peers. A failed close retains the handle so cleanup can be retried.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.lastErr == nil {
		d.lastErr = d.statusLocked().Error
	}

	return d.closeLocked()
}

// GetPeerStatuses returns the live peer state from the WireGuard device.
func (d *Device) GetPeerStatuses() (
	[]PeerStatus,
	error,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.backend == nil {
		return nil, fmt.Errorf("wireguard: device is stopped")
	}

	return d.backend.GetPeerStatuses()
}

// SetPeers replaces the desired peer set and immediately syncs the live
// WireGuard state to match. Each PeerConfig is parsed and validated
// before any changes are applied. The mutex is held across the entire
// observe→plan→apply cycle so that concurrent calls are serialized.
func (d *Device) SetPeers(
	configs ...PeerConfig,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	peers := make([]Peer, len(configs))
	for i, cfg := range configs {
		var err error
		peers[i], err = cfg.Parse()
		if err != nil {
			return fmt.Errorf("wireguard: peer config %d: %w", i, err)
		}
	}

	d.desired = make(map[wgtypes.Key]Peer, len(peers))
	for _, p := range peers {
		d.desired[p.PublicKey] = p
	}

	return d.syncPeersLocked(peers)
}

// SetPeerEndpoint updates the endpoint for a single existing peer,
// bypassing the full peer sync. It updates the desired
// entry's endpoint and applies a targeted op with the full peer config
// so that routing is preserved.
func (d *Device) SetPeerEndpoint(
	pubKey string,
	endpoint string,
) error {
	key, err := parseKey(pubKey)
	if err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	peer, ok := d.desired[key]
	if !ok {
		return fmt.Errorf("wireguard: update endpoint: peer %s not found", pubKey)
	}
	peer.Endpoint = addr
	d.desired[key] = peer

	if d.backend == nil {
		return fmt.Errorf("wireguard: device is stopped")
	}

	d.log.Debug(
		"peer op", "op", "set-endpoint",
		"peer", shortKey(key),
		"endpoint", endpoint,
	)

	return d.backend.ApplyPeerOps([]PeerOp{{Target: peer}})
}

// ValidateDeviceName checks that a device name fits within the kernel
// interface name length limit.
func ValidateDeviceName(
	name string,
) error {
	if len(name) > maxInterfaceNameBytes {
		return fmt.Errorf("wireguard: device name %q exceeds %d byte limit", name, maxInterfaceNameBytes)
	}
	return nil
}

// statusLocked requires mu. Reads never consume a retained lifecycle failure.
func (d *Device) statusLocked() resource.Status {
	if d.lastErr != nil {
		return resource.Status{
			State: resource.Failed,
			Error: d.lastErr,
		}
	}

	if d.backend == nil {
		return resource.Status{
			State: resource.Stopped,
		}
	}

	if err := d.backend.Check(); err != nil {
		return resource.Status{
			State: resource.Failed,
			Error: err,
		}
	}

	return resource.Status{
		State: resource.Running,
	}
}

// startLocked requires mu. A partial handle remains owned until cleanup succeeds.
func (d *Device) startLocked(
	ctx context.Context,
) (
	err error,
) {
	defer func() { d.lastErr = err }()

	d.backend, err = d.factory.CreateDevice(d.config)
	if err != nil {
		// The backend already attempted rollback. Retain any remaining handle
		// for a later Close or reconciliation instead of repeating cleanup here.
		return fmt.Errorf("wireguard: create device: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return errors.Join(err, d.closeLocked())
	}

	if d.desired != nil {
		peers := make([]Peer, 0, len(d.desired))
		for _, peer := range d.desired {
			peers = append(peers, peer)
		}
		if err := d.syncPeersLocked(peers); err != nil {
			return errors.Join(
				fmt.Errorf("wireguard: restore peers: %w", err),
				d.closeLocked(),
			)
		}
	}

	d.log.Debug("device started",
		"interface", d.backend.Name(),
		"route", d.config.Route.String(),
		"port", d.config.ListenPort,
	)
	return nil
}

// closeLocked requires mu. Only successful cleanup relinquishes the handle.
func (d *Device) closeLocked() error {
	if d.backend == nil {
		return nil
	}

	if err := d.backend.Close(); err != nil {
		err = fmt.Errorf("wireguard: close device: %w", err)
		d.lastErr = errors.Join(d.lastErr, err)
		return err
	}

	d.backend = nil
	return nil
}

// syncPeersLocked requires mu and syncs the live handle to the supplied desired peers.
func (d *Device) syncPeersLocked(
	peers []Peer,
) error {
	if d.backend == nil {
		return fmt.Errorf("wireguard: device is stopped")
	}

	observed, err := d.backend.GetPeerStatuses()
	if err != nil {
		return fmt.Errorf("wireguard: sync peers: read live state: %w", err)
	}

	ops := planPeerOps(peers, observed)
	if len(ops) == 0 {
		return nil
	}

	if d.log.Enabled(context.Background(), slog.LevelDebug) {
		d.logOps(observed, ops)
	}

	return d.backend.ApplyPeerOps(ops)
}

func (d *Device) logOps(
	observed []PeerStatus,
	ops []PeerOp,
) {

	// udpAddrString renders an optional endpoint for log lines.
	udpAddrString := func(addr *net.UDPAddr) string {
		if addr == nil {
			return ""
		}
		return addr.String()
	}

	live := make(map[wgtypes.Key]bool, len(observed))
	for _, p := range observed {
		live[p.PublicKey] = true
	}
	var added, removed, updated int
	for _, op := range ops {
		verb := "add"
		switch {
		case op.Remove:
			verb = "remove"
			removed++
		case live[op.Target.PublicKey]:
			verb = "update"
			updated++
		default:
			added++
		}
		d.log.Debug("peer op",
			"op", verb,
			"peer", shortKey(op.Target.PublicKey),
			"endpoint", udpAddrString(op.Target.Endpoint),
		)
	}
}

// shortKey renders a public key as a truncated prefix for log lines —
// enough to tell peers apart without drowning the output.
func shortKey(
	key wgtypes.Key,
) string {
	s := key.String()
	if len(s) > 8 {
		return s[:8] + "…"
	}
	return s
}

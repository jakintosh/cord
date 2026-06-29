package wireguard

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

const defaultMTU = 1420

// wgDevice implements WGDevice backed by a real WireGuard backend.
type wgDevice struct {
	name       string
	privateKey wgtypes.Key
	address    net.IPNet
	port       uint16
	mtu        int
	noRoutes   bool
	up         bool

	backend  Backend
	realName string // actual OS device name (e.g. utun4 on macOS)

	desired map[wgtypes.Key]desiredPeer

	status ReconcileStatus
	logf   func(format string, args ...any)

	mu sync.Mutex
}

func newDevice(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	port uint16,
	mtu int,
	noRoutes bool,
	backend Backend,
) *wgDevice {
	if mtu <= 0 {
		mtu = defaultMTU
	}
	return &wgDevice{
		name:       name,
		privateKey: privateKey,
		address:    address,
		port:       port,
		mtu:        mtu,
		noRoutes:   noRoutes,
		backend:    backend,
		desired:    make(map[wgtypes.Key]desiredPeer),
	}
}

// DeviceName returns the actual OS device name once the interface is
// up. Before Up() (and everywhere on Linux) it is the configured name.
func (d *wgDevice) DeviceName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.realName != "" {
		return d.realName
	}
	return d.name
}

// setRealName records the OS-assigned device name.
func (d *wgDevice) setRealName(
	name string,
) {
	d.mu.Lock()
	d.realName = name
	d.mu.Unlock()
}

// ApplyPeers stores the full desired peer set and reconciles it
// against the live WireGuard device.
func (d *wgDevice) ApplyPeers(
	peers []WGPeer,
) error {
	d.mu.Lock()
	desired, err := buildDesiredPeers(peers)
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("wireguard: build desired peers: %w", err)
	}
	d.desired = desired
	up := d.up
	d.mu.Unlock()

	if !up {
		return ErrDeviceNotUp
	}
	return d.reconcile()
}

// UpdateEndpoint sets the endpoint for a single existing peer,
// bypassing the normal reconciliation flow. It builds a targeted
// PeerUpdate operation and applies it directly to the backend.
func (d *wgDevice) UpdateEndpoint(pubKey, endpoint string) error {
	d.mu.Lock()
	up := d.up
	d.mu.Unlock()
	if !up {
		return ErrDeviceNotUp
	}

	key, err := parseKey(pubKey)
	if err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	op := PeerOperation{
		Type:           PeerUpdate,
		Peer:           desiredPeer{PublicKey: key, Endpoint: addr},
		UpdateEndpoint: true,
	}

	devName := d.deviceName()
	if err := d.backend.ApplyPeerOperations(devName, []PeerOperation{op}); err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	// Update the desired set so future reconciliations don't revert.
	d.mu.Lock()
	if dp, ok := d.desired[key]; ok {
		dp.Endpoint = addr
		d.desired[key] = dp
	}
	d.mu.Unlock()

	return nil
}

// Up creates the network device, configures it, and brings it up.
// An initial reconciliation is performed after the device is up.
func (d *wgDevice) Up() error {
	d.mu.Lock()
	cfg := DeviceConfig{
		Name:       d.name,
		PrivateKey: d.privateKey,
		Address:    d.address,
		ListenPort: int(d.port),
		MTU:        d.mtu,
		NoRoutes:   d.noRoutes,
	}

	if err := d.backend.Up(cfg); err != nil {
		d.mu.Unlock()
		return err
	}
	d.up = true
	d.mu.Unlock()

	if err := d.reconcile(); err != nil {
		d.verboselog("up: initial reconciliation failed: %v", err)
	}
	return nil
}

// Down brings the device down without removing it.
func (d *wgDevice) Down() error {
	d.mu.Lock()
	d.up = false
	d.mu.Unlock()
	return d.backend.Down(d.deviceName())
}

// WaitForHandshake polls the live device until the named peer
// completes a handshake or the timeout expires.
func (d *wgDevice) WaitForHandshake(
	pubKey string,
	timeout time.Duration,
	onStatus func(PeerStatus),
) error {
	d.mu.Lock()
	up := d.up
	d.mu.Unlock()
	if !up {
		return ErrDeviceNotUp
	}

	findPeer := func(
		status *DeviceStatus,
		key wgtypes.Key,
	) (
		PeerStatus,
		bool,
	) {
		for _, peer := range status.Peers {
			if peer.PublicKey == key {
				return PeerStatus{
					PublicKey:     peer.PublicKey.String(),
					Endpoint:      endpointString(peer.Endpoint),
					LastHandshake: peer.LastHandshake,
					ReceiveBytes:  peer.ReceiveBytes,
					TransmitBytes: peer.TransmitBytes,
				}, true
			}
		}
		return PeerStatus{}, false
	}

	key, err := parseKey(pubKey)
	if err != nil {
		return fmt.Errorf("wireguard: wait for handshake: %w", err)
	}

	devName := d.deviceName()
	deadline := time.Now().Add(timeout)

	for true {
		status, err := d.backend.Status(devName)
		if err == nil {
			if ps, ok := findPeer(status, key); ok {
				if onStatus != nil {
					onStatus(ps)
				}
				if !ps.LastHandshake.IsZero() {
					break
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
		if time.Now().After(deadline) {
			return fmt.Errorf("wireguard: no handshake completed within %s", timeout)
		}
	}

	return nil
}

// Status returns the live WireGuard device state by querying the
// backend, converting observed peer state into PeerStatus values.
func (d *wgDevice) Status() (
	[]PeerStatus,
	error,
) {
	d.mu.Lock()
	up := d.up
	d.mu.Unlock()
	if !up {
		return nil, ErrDeviceNotUp
	}

	devName := d.deviceName()
	status, err := d.backend.Status(devName)
	if err != nil {
		return nil, fmt.Errorf("wireguard: status: %w", err)
	}

	peers := make([]PeerStatus, len(status.Peers))
	for i, p := range status.Peers {
		peers[i] = PeerStatus{
			PublicKey:     p.PublicKey.String(),
			Endpoint:      endpointString(p.Endpoint),
			LastHandshake: p.LastHandshake,
			ReceiveBytes:  p.ReceiveBytes,
			TransmitBytes: p.TransmitBytes,
		}
	}
	return peers, nil
}

// SetLogger configures optional logging for reconciliation activity.
func (d *wgDevice) SetLogger(logf func(format string, args ...any)) {
	d.mu.Lock()
	d.logf = logf
	d.mu.Unlock()
}

// ReconcileStatus returns the latest reconciliation attempt state.
func (d *wgDevice) ReconcileStatus() ReconcileStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

func (d *wgDevice) deviceName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.realName != "" {
		return d.realName
	}
	return d.name
}

func (d *wgDevice) verboselog(format string, args ...any) {
	d.mu.Lock()
	logf := d.logf
	d.mu.Unlock()
	if logf != nil {
		logf(format, args...)
	}
}

// reconcile observes the live device, plans changes, and applies
// only the operations needed to match the desired peer set.
func (d *wgDevice) reconcile() error {
	d.mu.Lock()
	desired := d.desired
	backend := d.backend
	name := d.realName
	if name == "" {
		name = d.name
	}
	d.mu.Unlock()

	now := time.Now()
	desiredSlice := make([]desiredPeer, 0, len(desired))
	for _, p := range desired {
		desiredSlice = append(desiredSlice, p)
	}

	d.verboselog("reconciliation started: interface=%s desired=%d", name, len(desiredSlice))

	devStatus, err := backend.Status(name)
	if err != nil {
		d.mu.Lock()
		d.status.LastAttempt = now
		d.status.Desired = len(desiredSlice)
		d.status.Observed = 0
		d.status.Error = &ReconcileError{Stage: StageObserve, Message: err.Error()}
		d.mu.Unlock()
		d.verboselog("reconciliation failed: interface=%s stage=observe error=%v", name, err)
		return fmt.Errorf("wireguard: reconcile observe: %w", err)
	}

	plan := PlanPeerReconciliation(desiredSlice, devStatus.Peers)
	d.mu.Lock()
	d.status.LastAttempt = now
	d.status.Desired = len(desiredSlice)
	d.status.Observed = len(devStatus.Peers)
	d.status.Error = nil
	d.mu.Unlock()

	if len(plan.Operations) == 0 {
		d.mu.Lock()
		d.status.LastSuccess = now
		d.mu.Unlock()
		return nil
	}

	adds, updates, removes := plan.OperationCounts()
	d.verboselog(
		"reconciliation planned: interface=%s observed=%d add=%d update=%d remove=%d",
		name,
		len(devStatus.Peers),
		adds,
		updates,
		removes,
	)

	for _, op := range plan.Operations {
		d.verboselog(
			"reconciliation operation: interface=%s type=%s peer=%s fields=%s",
			name,
			op.Type,
			shortKey(op.Peer.PublicKey),
			op.Fields(),
		)
	}

	if err := backend.ApplyPeerOperations(name, plan.Operations); err != nil {
		d.mu.Lock()
		d.status.Error = &ReconcileError{Stage: StageApply, Message: err.Error()}
		d.mu.Unlock()
		d.verboselog("reconciliation failed: interface=%s stage=apply error=%v", name, err)
		return fmt.Errorf("wireguard: reconcile apply: %w", err)
	}

	d.mu.Lock()
	d.status.LastSuccess = now
	d.mu.Unlock()
	d.verboselog("reconciliation applied: interface=%s operations=%d", name, len(plan.Operations))
	return nil
}

// buildDesiredPeers converts WGPeer values to internal desiredPeer
// values, parsing keys and addresses from their string representations.
func buildDesiredPeers(
	peers []WGPeer,
) (
	map[wgtypes.Key]desiredPeer,
	error,
) {
	result := make(map[wgtypes.Key]desiredPeer, len(peers))
	for _, p := range peers {
		key, err := parseKey(p.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("public key %q: %w", p.PublicKey, err)
		}

		var allowedIPs []net.IPNet
		for _, cidr := range p.AllowedIPs {
			ipNet, err := netaddr.ParseInterface(cidr)
			if err != nil {
				return nil, fmt.Errorf("allowed-ip %q: %w", cidr, err)
			}
			allowedIPs = append(allowedIPs, ipNet)
		}

		var endpoint *net.UDPAddr
		if p.Endpoint != "" {
			ep, err := net.ResolveUDPAddr("udp", p.Endpoint)
			if err != nil {
				return nil, fmt.Errorf("endpoint %q: %w", p.Endpoint, err)
			}
			endpoint = ep
		}

		result[key] = desiredPeer{
			PublicKey:           key,
			AllowedIPs:          allowedIPs,
			Endpoint:            endpoint,
			EndpointPolicy:      p.EndpointPolicy,
			PersistentKeepalive: time.Duration(p.PersistentKeepalive) * time.Second,
		}
	}
	return result, nil
}

func endpointString(ep *net.UDPAddr) string {
	if ep == nil {
		return ""
	}
	return ep.String()
}

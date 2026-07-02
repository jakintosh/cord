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

// Peer is a single WireGuard peer, used for both desired configuration
// and observed live state. Runtime fields (LastHandshake, ReceiveBytes,
// TransmitBytes) are zero when used as desired config.
type Peer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	EndpointPolicy      EndpointPolicy
	PersistentKeepalive time.Duration
	LastHandshake       time.Time
	ReceiveBytes        int64
	TransmitBytes       int64
}

// NewPeer parses string representations into a Peer. endpoint may be
// empty. keepaliveSec is seconds; 0 means no keepalive.
func NewPeer(
	publicKey string,
	allowedIPs []string,
	endpoint string,
	keepaliveSec int,
	policy EndpointPolicy,
) (
	Peer,
	error,
) {
	key, err := parseKey(publicKey)
	if err != nil {
		return Peer{}, fmt.Errorf("public key %q: %w", publicKey, err)
	}

	var ips []net.IPNet
	for _, cidr := range allowedIPs {
		ipNet, err := netaddr.ParseInterface(cidr)
		if err != nil {
			return Peer{}, fmt.Errorf("allowed-ip %q: %w", cidr, err)
		}
		ips = append(ips, ipNet)
	}

	var ep *net.UDPAddr
	if endpoint != "" {
		ep, err = net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			return Peer{}, fmt.Errorf("endpoint %q: %w", endpoint, err)
		}
	}

	return Peer{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      policy,
		PersistentKeepalive: time.Duration(keepaliveSec) * time.Second,
	}, nil
}

// Device is a live WireGuard network device backed by a Backend.
// ApplyPeers replaces the set of desired peers and reconciles them
// against the live WireGuard state.
type Device struct {
	name       string
	privateKey wgtypes.Key
	address    net.IPNet
	port       uint16
	mtu        int
	noRoutes   bool
	up         bool

	backend Backend
	desired map[wgtypes.Key]Peer

	status ReconcileStatus
	logf   func(format string, args ...any)

	mu          sync.Mutex
	reconcileMu sync.Mutex
}

func newDevice(
	name string,
	privateKey wgtypes.Key,
	address net.IPNet,
	port uint16,
	mtu int,
	noRoutes bool,
	backend Backend,
) *Device {
	if mtu <= 0 {
		mtu = defaultMTU
	}
	return &Device{
		name:       name,
		privateKey: privateKey,
		address:    address,
		port:       port,
		mtu:        mtu,
		noRoutes:   noRoutes,
		backend:    backend,
		desired:    make(map[wgtypes.Key]Peer),
	}
}

// Name returns the device name.
func (d *Device) Name() string {
	return d.name
}

// Up creates the network device, configures it, and brings it up.
// An initial reconciliation is performed after the device is up.
func (d *Device) Up() error {
	d.mu.Lock()

	if err := d.backend.Up(
		d.name,
		d.privateKey,
		d.address,
		int(d.port),
		d.mtu,
		d.noRoutes,
	); err != nil {
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
func (d *Device) Down() error {
	d.mu.Lock()
	d.up = false
	d.mu.Unlock()
	return d.backend.Down(d.name)
}

// GetPeers returns the live peer state from the WireGuard device.
func (d *Device) GetPeers() (
	[]Peer,
	error,
) {
	d.mu.Lock()
	up := d.up
	d.mu.Unlock()
	if !up {
		return nil, ErrDeviceNotUp
	}

	peers, err := d.backend.GetPeers(d.name)
	if err != nil {
		return nil, fmt.Errorf("wireguard: peers: %w", err)
	}
	return peers, nil
}

// SetPeers stores the full desired peer set and reconciles it
// against the live WireGuard device.
func (d *Device) SetPeers(
	peers ...Peer,
) error {
	d.mu.Lock()
	desired := make(map[wgtypes.Key]Peer, len(peers))
	for _, p := range peers {
		desired[p.PublicKey] = p
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
func (d *Device) UpdateEndpoint(
	pubKey string,
	endpoint string,
) error {
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

	d.reconcileMu.Lock()
	defer d.reconcileMu.Unlock()

	op := PeerOperation{
		Type: PeerUpdate,
		Peer: Peer{
			PublicKey: key,
			Endpoint:  addr,
		},
		UpdateEndpoint: true,
	}

	if err := d.backend.ModifyPeers(d.name, []PeerOperation{op}); err != nil {
		return fmt.Errorf("wireguard: update endpoint: %w", err)
	}

	d.mu.Lock()
	if dp, ok := d.desired[key]; ok {
		dp.Endpoint = addr
		d.desired[key] = dp
	}
	d.mu.Unlock()

	return nil
}

// SetLogger configures optional logging for reconciliation activity.
func (d *Device) SetLogger(logf func(format string, args ...any)) {
	d.mu.Lock()
	d.logf = logf
	d.mu.Unlock()
}

// ReconcileStatus returns the latest reconciliation attempt state.
func (d *Device) ReconcileStatus() ReconcileStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

func (d *Device) verboselog(format string, args ...any) {
	d.mu.Lock()
	logf := d.logf
	d.mu.Unlock()
	if logf != nil {
		logf(format, args...)
	}
}

// reconcile observes the live device, plans changes, and applies
// only the operations needed to match the desired peer set.
func (d *Device) reconcile() error {
	d.reconcileMu.Lock()
	defer d.reconcileMu.Unlock()

	d.mu.Lock()
	desired := d.desired
	backend := d.backend
	name := d.name
	d.mu.Unlock()

	now := time.Now()
	desiredSlice := make([]Peer, 0, len(desired))
	for _, p := range desired {
		desiredSlice = append(desiredSlice, p)
	}

	d.verboselog("reconciliation started: interface=%s desired=%d", name, len(desiredSlice))

	devObserved, err := backend.GetPeers(name)
	if err != nil {
		d.mu.Lock()
		d.status.LastAttempt = now
		d.status.Desired = len(desiredSlice)
		d.status.Observed = 0
		d.status.Error = &ReconcileError{
			Stage:   StageObserve,
			Message: err.Error(),
		}
		d.mu.Unlock()
		d.verboselog("reconciliation failed: interface=%s stage=observe error=%v", name, err)
		return fmt.Errorf("wireguard: reconcile observe: %w", err)
	}

	plan := PlanPeerReconciliation(desiredSlice, devObserved)
	d.mu.Lock()
	d.status.LastAttempt = now
	d.status.Desired = len(desiredSlice)
	d.status.Observed = len(devObserved)
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
		len(devObserved),
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

	if err := backend.ModifyPeers(name, plan.Operations); err != nil {
		d.mu.Lock()
		d.status.Error = &ReconcileError{
			Stage:   StageApply,
			Message: err.Error(),
		}
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

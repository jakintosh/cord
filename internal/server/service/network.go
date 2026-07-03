package service

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// defaultInviteCidr is the invite subnet used when CreateNetwork is
// called without an explicit Invite.Cidr.
const defaultInviteCidr = "172.16.10.0/24"

// inviteSuffix is the suffix appended to the network name for the
// invite device.
const inviteSuffix = "-i"

// defaultReconcileCap is the maximum duration between reconciliation
// passes. The self-rearming timer uses min(earliest ExpiresAt, now+cap).
const defaultReconcileCap = 5 * time.Minute

// NetworkConfig is the persisted identity of a server network. It holds
// the server's keypair, address space configuration, and network
// endpoints. It is inert domain data — the runtime Network owns all
// behavior.
type NetworkConfig struct {
	Name       string
	PrivateKey string
	PublicKey  string
	ExternalIP string

	Main   PlaneConfig
	Invite PlaneConfig

	Enabled   bool
	CreatedAt time.Time
}

// Normalize applies defaults and validates the config. Zero-valued
// fields are filled with sensible defaults. Returns ErrInvalidInput
// or ErrCIDROverlap on failure.
func (nc *NetworkConfig) Normalize() error {
	if nc.Name == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}
	if nc.ExternalIP == "" {
		return fmt.Errorf("%w: external IP required", ErrInvalidInput)
	}

	// Main plane defaults
	if nc.Main.Name == "" {
		nc.Main.Name = nc.Name
	}
	if nc.Main.WireguardPort == 0 {
		nc.Main.WireguardPort = 51820
	}
	if nc.Main.ApiPort == 0 {
		nc.Main.ApiPort = 80
	}

	// Invite plane defaults
	if nc.Invite.Name == "" {
		nc.Invite.Name = nc.Name + inviteSuffix
	}
	if nc.Invite.Cidr == "" {
		nc.Invite.Cidr = defaultInviteCidr
	}
	if nc.Invite.WireguardPort == 0 {
		nc.Invite.WireguardPort = nc.Main.WireguardPort + 1
	}
	if nc.Invite.ApiPort == 0 {
		nc.Invite.ApiPort = 80
	}

	// Validate main plane
	if err := nc.Main.validate(); err != nil {
		return fmt.Errorf("main: %w", err)
	}

	// Validate invite plane
	if err := nc.Invite.validate(); err != nil {
		return fmt.Errorf("invite: %w", err)
	}

	// Cross-plane overlap check
	_, mainNet, err := net.ParseCIDR(nc.Main.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid main CIDR %q: %v", ErrInvalidInput, nc.Main.Cidr, err)
	}
	_, inviteNet, err := net.ParseCIDR(nc.Invite.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid invite CIDR %q: %v", ErrInvalidInput, nc.Invite.Cidr, err)
	}
	if netaddr.Overlaps(mainNet, inviteNet) {
		return fmt.Errorf(
			"%w: invite CIDR %q overlaps main CIDR %q",
			ErrCIDROverlap, nc.Invite.Cidr, nc.Main.Cidr,
		)
	}

	return nil
}

// Network is a running server network: two Planes (main + invite) plus
// a self-rearming reconciliation timer.
type Network struct {
	config   *NetworkConfig
	main     *Plane
	invite   *Plane
	timer    *time.Timer
	store    Store
	wg       *wireguard.Manager
	api      func(network string) APIHandlers
	clock    func() time.Time
	logf     func(string, ...any)
	registry func() map[string]*Network
}

// GetNetwork returns the persisted network config by name.
func (s *Service) GetNetwork(
	name string,
) (
	*NetworkConfig,
	error,
) {
	nc, err := s.store.GetNetwork(name)
	if err != nil {
		return nil, fmt.Errorf("get network %q: %w", name, mapStoreError(err))
	}
	return nc, nil
}

// ListNetworks returns the names of all persisted server networks.
func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	names, err := s.store.ListNetworkNames()
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return names, nil
}

// CreateNetwork defines a new server network: validates the config,
// generates the server keypair, and persists the record. Zero-valued
// fields in the PlaneConfigs mean "use default."
func (s *Service) CreateNetwork(
	name string,
	externalIP string,
	main PlaneConfig,
	invite PlaneConfig,
) (
	*NetworkConfig,
	error,
) {
	privKey, err := wireguard.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	pubKey, err := wireguard.PublicKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}

	nc := &NetworkConfig{
		Name:       name,
		PrivateKey: privKey,
		PublicKey:  pubKey,
		ExternalIP: externalIP,

		Main:   main,
		Invite: invite,

		Enabled:   false,
		CreatedAt: s.clock(),
	}
	if err := nc.Normalize(); err != nil {
		return nil, err
	}

	_, mainNet, err := net.ParseCIDR(nc.Main.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse main CIDR: %w", err)
	}
	ones, bits := mainNet.Mask.Size()
	rootCidr := &Cidr{
		Name:   name,
		Cidr:   nc.Main.Cidr,
		Prefix: ones,
		Bits:   bits,
	}

	serverIP := netaddr.FirstAssignable(mainNet)
	serverCIDR := netaddr.HostRoute(serverIP)
	serverPeer := &Peer{
		Name:      "cord-server",
		Cidr:      serverCIDR.String(),
		PublicKey: pubKey,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}

	if err := s.store.BootstrapNetwork(nc, rootCidr, serverPeer); err != nil {
		return nil, fmt.Errorf("bootstrap network: %w", mapStoreError(err))
	}

	return nc, nil
}

// DeleteNetwork removes the named network and all of its resources.
// The network must be stopped first.
func (s *Service) DeleteNetwork(
	name string,
) error {
	s.mu.Lock()
	_, running := s.running[name]
	s.mu.Unlock()

	if running {
		return fmt.Errorf("%w: %s", ErrNetworkRunning, name)
	}

	if err := s.store.DeleteNetwork(name); err != nil {
		return fmt.Errorf("delete network %q: %w", name, mapStoreError(err))
	}
	return nil
}

// EnableNetwork persists the enabled flag and starts the network.
// Idempotent: if the network is already running this is a no-op.
func (s *Service) EnableNetwork(
	name string,
) error {
	if err := s.store.SetNetworkEnabled(name, true); err != nil {
		return fmt.Errorf("enable network %q: %w", name, mapStoreError(err))
	}

	s.mu.Lock()
	_, exists := s.running[name]
	s.mu.Unlock()
	if exists {
		return nil
	}

	return s.startNetwork(name)
}

// DisableNetwork stops the network and persists the disabled flag.
// Idempotent.
func (s *Service) DisableNetwork(
	name string,
) error {
	s.mu.Lock()
	n, exists := s.running[name]
	s.mu.Unlock()

	if exists {
		if err := n.stop(); err != nil {
			return fmt.Errorf("disable network %q: %w", name, err)
		}
		s.mu.Lock()
		delete(s.running, name)
		s.mu.Unlock()
	}

	if err := s.store.SetNetworkEnabled(name, false); err != nil {
		return fmt.Errorf("disable network %q: %w", name, mapStoreError(err))
	}
	return nil
}

// reconcile triggers a synchronous reconciliation for the named
// network. Used by mutation paths (peer updates, registration
// operations) to apply changes immediately.
func (s *Service) reconcile(
	name string,
) {
	s.mu.Lock()
	n, exists := s.running[name]
	s.mu.Unlock()

	if exists {
		n.reconcile()
	}
}

// start brings up both planes and performs the initial reconciliation.
func (n *Network) start() error {

	var mainHandler, inviteHandler http.Handler
	if n.api != nil {
		h := n.api(n.config.Name)
		mainHandler = h.Main
		inviteHandler = h.Invite
	}

	n.main = newPlane(n.config.Main, n.config.PrivateKey)
	if err := n.main.start(n.wg, mainHandler); err != nil {
		return fmt.Errorf("main plane: %w", err)
	}

	n.invite = newPlane(n.config.Invite, n.config.PrivateKey)
	if err := n.invite.start(n.wg, inviteHandler); err != nil {
		_ = n.main.stop()
		return fmt.Errorf("invite plane: %w", err)
	}

	n.reconcile()
	return nil
}

// stop stops the timer, then stops both planes. Errors are joined.
func (n *Network) stop() error {
	if n.timer != nil {
		n.timer.Stop()
	}

	var errs []error
	if n.main != nil {
		if err := n.main.stop(); err != nil {
			errs = append(errs, err)
		}
	}
	if n.invite != nil {
		if err := n.invite.stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// reconcile is the ONLY code path that writes peer state to devices.
// It prunes expired registrations, builds peer sets for both planes,
// applies them, and rearms the timer.
func (n *Network) reconcile() {
	netName := n.config.Name
	now := n.clock()

	if err := n.store.PruneExpiredRegistrations(netName, now); err != nil {
		n.logf("reconcile %s: prune: %v", netName, err)
		return
	}

	// reconcile main peers
	peers, err := n.store.ListPeers(netName)
	if err != nil {
		n.logf("reconcile %s: list peers: %w", netName, err)
		return
	}
	mainPeers, err := peersToWireGuardPeers(peers)
	if err != nil {
		n.logf("reconcile %s: build main peers: %v", netName, err)
		return
	}
	if err := n.main.device.SetPeers(mainPeers...); err != nil {
		n.logf("reconcile %s: apply main peers: %v", netName, err)
	}

	// reconcile invite peers
	regs, err := n.store.ListActiveRegistrations(netName, now)
	if err != nil {
		n.logf("reconcile %s: list active registrations: %w", netName, err)
		return
	}
	invitePeers, err := registrationsToWireGuardPeers(regs)
	if err != nil {
		n.logf("reconcile %s: build invite peers: %v", netName, err)
		return
	}
	if err := n.invite.device.SetPeers(invitePeers...); err != nil {
		n.logf("reconcile %s: apply invite peers: %v", netName, err)
	}

	// Rearm timer
	next := now.Add(defaultReconcileCap)
	for _, reg := range regs {
		if reg.ExpiresAt.Before(next) {
			next = reg.ExpiresAt
		}
	}
	delay := next.Sub(now)
	if delay <= 0 {
		delay = time.Second
	}
	if n.timer == nil {
		n.timer = time.AfterFunc(delay, n.reconcile)
	} else {
		n.timer.Reset(delay)
	}
}

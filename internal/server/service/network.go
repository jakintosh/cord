package service

import (
	"errors"
	"fmt"
	"log/slog"
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
func (n *NetworkConfig) Normalize() error {
	if n.Name == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}
	if n.ExternalIP == "" {
		return fmt.Errorf("%w: external IP required", ErrInvalidInput)
	}

	// Main plane defaults
	if n.Main.Name == "" {
		n.Main.Name = n.Name
	}
	if n.Main.WireguardPort == 0 {
		n.Main.WireguardPort = 51820
	}
	if n.Main.ApiPort == 0 {
		n.Main.ApiPort = 8080
	}

	// Invite plane defaults
	if n.Invite.Name == "" {
		n.Invite.Name = n.Name + inviteSuffix
	}
	if n.Invite.Cidr == "" {
		n.Invite.Cidr = defaultInviteCidr
	}
	if n.Invite.WireguardPort == 0 {
		n.Invite.WireguardPort = n.Main.WireguardPort + 1
	}
	if n.Invite.ApiPort == 0 {
		n.Invite.ApiPort = 8080
	}

	// Validate main plane
	if err := n.Main.validate(); err != nil {
		return fmt.Errorf("main: %w", err)
	}

	// Validate invite plane
	if err := n.Invite.validate(); err != nil {
		return fmt.Errorf("invite: %w", err)
	}

	// Cross-plane overlap check
	_, mainNet, err := net.ParseCIDR(n.Main.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid main CIDR %q: %v", ErrInvalidInput, n.Main.Cidr, err)
	}
	_, inviteNet, err := net.ParseCIDR(n.Invite.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid invite CIDR %q: %v", ErrInvalidInput, n.Invite.Cidr, err)
	}
	if netaddr.Overlaps(mainNet, inviteNet) {
		return fmt.Errorf(
			"%w: invite CIDR %q overlaps main CIDR %q",
			ErrCIDROverlap, n.Invite.Cidr, n.Main.Cidr,
		)
	}

	return nil
}

// Network is a running server network: two Planes (main + invite) plus
// a self-rearming reconciliation timer.
type Network struct {
	config *NetworkConfig
	main   *Plane
	invite *Plane
	timer  *time.Timer
	store  Store
	wg     *wireguard.Manager
	api    func(network string) APIHandlers
	clock  func() time.Time
	log    *slog.Logger
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
	serverRoute := netaddr.HostRoute(serverIP)
	serverPeer := &Peer{
		Name:      "cord-server",
		Route:     serverRoute.String(),
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

	n.main = newPlane(
		n.config.Main,
		n.config.PrivateKey,
		n.log.With("plane", "main"),
	)
	if err := n.main.start(n.wg, mainHandler); err != nil {
		return fmt.Errorf("main plane: %w", err)
	}

	n.invite = newPlane(
		n.config.Invite,
		n.config.PrivateKey,
		n.log.With("plane", "invite"),
	)
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
	n.log.Info("network stopped")
	return errors.Join(errs...)
}

// reconcile is the ONLY code path that writes peer state to devices.
// It prunes expired registrations, builds peer sets for both planes,
// applies them, and rearms the timer.
func (n *Network) reconcile() {
	netName := n.config.Name
	now := n.clock()

	if err := n.store.PruneExpiredRegistrations(netName, now); err != nil {
		n.log.Warn("reconcile: prune failed", "err", err)
		return
	}
	if err := n.store.DeleteEndpointsBefore(netName, now.Add(-defaultEndpointTTL)); err != nil {
		n.log.Warn("reconcile: prune endpoints failed", "err", err)
	}

	// reconcile main peers
	peers, err := n.store.ListPeers(netName)
	if err != nil {
		n.log.Warn("reconcile: list peers failed", "err", err)
		return
	}
	mainPeers, err := peersToWireGuardPeers(peers)
	if err != nil {
		n.log.Warn("reconcile: build main peers failed", "err", err)
		return
	}
	if err := n.main.device.SetPeers(mainPeers...); err != nil {
		n.log.Warn("reconcile: apply main peers failed", "err", err)
	} else {
		n.observeMainPeerEndpoints(peers, now)
	}

	// reconcile invite peers
	regs, err := n.store.ListActiveRegistrations(netName, now)
	if err != nil {
		n.log.Warn("reconcile: list active registrations failed", "err", err)
		return
	}
	invitePeers := registrationsToWireGuardPeers(regs)
	if err := n.invite.device.SetPeers(invitePeers...); err != nil {
		n.log.Warn("reconcile: apply invite peers failed", "err", err)
	}
	n.log.Debug("reconcile", "peers", len(peers), "registrations", len(regs))

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

// observeMainPeerEndpoints records recent endpoints learned by the server's
// main WireGuard device. This provides the initial endpoint candidates clients
// need before they can make direct peer-to-peer handshakes themselves.
func (n *Network) observeMainPeerEndpoints(
	peers []*Peer,
	now time.Time,
) {
	livePeers, err := n.main.device.Peers()
	if err != nil {
		n.log.Warn("reconcile: observe main peer endpoints failed", "err", err)
		return
	}

	known := make(map[string]struct{}, len(peers))
	for _, peer := range peers {
		if peer.PublicKey == n.config.PublicKey || !peer.Enabled || !peer.Confirmed {
			continue
		}
		known[peer.PublicKey] = struct{}{}
	}

	sightings := make([]EndpointSighting, 0, len(livePeers))
	for _, peer := range livePeers {
		if _, ok := known[peer.PublicKey.String()]; !ok {
			continue
		}
		if peer.Endpoint == nil || peer.LastHandshake.IsZero() ||
			now.Sub(peer.LastHandshake) >= wireguard.ActiveHandshakeThreshold {
			continue
		}
		sightings = append(sightings, EndpointSighting{
			WitnessKey: n.config.PublicKey,
			PeerKey:    peer.PublicKey.String(),
			Endpoint:   peer.Endpoint.String(),
			Timestamp:  now,
		})
	}

	if len(sightings) == 0 {
		return
	}
	if err := n.store.InsertEndpointSightings(n.config.Name, sightings); err != nil {
		n.log.Warn("reconcile: store main peer endpoints failed", "err", err)
	}
}

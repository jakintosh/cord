package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Network is the persistent identity of a server network. It holds the
// server's keypair, address space configuration, and network endpoints.
// It is inert domain data — the Service owns all behavior.
type Network struct {
	Name string

	// Wireguard Entrypoint
	PrivateKey string
	PublicKey  string
	ExternalIP string // the server's public IP address

	// Main "Network"
	MainName          string // WireGuard interface name for the main device (defaults to Name)
	MainCidr          string // e.g. "10.42.0.0/16" — the main interface subnet
	MainWireguardPort uint16 // WireGuard listen port for the main interface
	MainApiPort       uint16 // the internal API port served over the tunnel

	// Invite "Network"
	InviteName          string // WireGuard interface name for the invite device (defaults to Name + "-i")
	InviteCidr          string // e.g. "10.43.0.0/24" — the invite interface subnet
	InviteWireguardPort uint16 // WireGuard listen port for the invite interface
	InviteApiPort       uint16 // the internal API port served over the invite tunnel

	// Metadata
	Enabled   bool      // whether the network's WireGuard devices are running
	CreatedAt time.Time // when the network was created
}

// GetNetwork returns the persisted network record by name.
// Returns ErrNotFound if the network does not exist.
func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	network, err := s.store.GetNetwork(name)
	if err != nil {
		return nil, fmt.Errorf("get network %q: %w", name, mapStoreError(err))
	}
	return network, nil
}

// ListNetworks returns the names of all persisted server networks,
// ordered alphabetically.
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
// generates the server keypair, and persists the record. It does not
// bring interfaces up — call StartNetwork for that.
//
// On success the returned Network includes the generated keys. Returns
// ErrNetworkExists if the name is already taken, ErrInvalidInput if
// the configuration is invalid.
func (s *Service) CreateNetwork(
	name string,
	externalIP string,
	mainCidr string,
	mainName *string,
	mainWireguardPort *uint16,
	mainApiPort *uint16,
	inviteName *string,
	inviteCidr *string,
	inviteWireguardPort *uint16,
	inviteApiPort *uint16,
) (
	*Network,
	error,
) {
	// validate network config
	if name == "" {
		return nil, fmt.Errorf("%w: network name required", ErrInvalidInput)
	}
	if externalIP == "" {
		return nil, fmt.Errorf("%w: external IP required", ErrInvalidInput)
	}

	// validate main interface config
	mainDevName := name
	if mainName != nil && *mainName != "" {
		mainDevName = *mainName
	}
	if err := wireguard.ValidateDeviceName(mainDevName); err != nil {
		return nil, fmt.Errorf("%w: invalid main device name: %v", ErrInvalidInput, err)
	}
	_, mainNet, err := net.ParseCIDR(mainCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid main CIDR %q: %v", ErrInvalidInput, mainCidr, err)
	}
	mainWgPort := uint16(51820)
	if mainWireguardPort != nil && *mainWireguardPort != 0 {
		mainWgPort = *mainWireguardPort
	}
	mainAPIPort := uint16(80)
	if mainApiPort != nil && *mainApiPort != 0 {
		mainAPIPort = *mainApiPort
	}

	// validate invite interface config
	inviteDevName := name + inviteSuffix
	if inviteName != nil && *inviteName != "" {
		inviteDevName = *inviteName
	}
	if err := wireguard.ValidateDeviceName(inviteDevName); err != nil {
		return nil, fmt.Errorf("%w: invalid invite device name: %v", ErrInvalidInput, err)
	}
	inviteCIDR := defaultInviteCidr
	if inviteCidr != nil && *inviteCidr != "" {
		inviteCIDR = *inviteCidr
	}
	_, inviteNet, err := net.ParseCIDR(inviteCIDR)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite CIDR %q: %v", ErrInvalidInput, inviteCIDR, err)
	}
	inviteWgPort := mainWgPort + 1
	if inviteWireguardPort != nil && *inviteWireguardPort != 0 {
		inviteWgPort = *inviteWireguardPort
	}
	inviteAPIPort := uint16(80)
	if inviteApiPort != nil && *inviteApiPort != 0 {
		inviteAPIPort = *inviteApiPort
	}

	if netaddr.Overlaps(mainNet, inviteNet) {
		return nil, fmt.Errorf(
			"%w: invite CIDR %q overlaps main CIDR %q",
			ErrCIDROverlap, inviteCIDR, mainCidr,
		)
	}

	privKey, err := s.wg.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	pubKey, err := s.wg.PublicKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}

	ones, bits := mainNet.Mask.Size()
	rootCidr := &Cidr{
		Name:   name,
		Cidr:   mainCidr,
		Length: ones,
		Prefix: bits,
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

	network := &Network{
		Name:                name,
		ExternalIP:          externalIP,
		PrivateKey:          privKey,
		PublicKey:           pubKey,
		MainName:            mainDevName,
		MainCidr:            mainCidr,
		MainWireguardPort:   mainWgPort,
		MainApiPort:         mainAPIPort,
		InviteName:          inviteDevName,
		InviteCidr:          inviteCIDR,
		InviteWireguardPort: inviteWgPort,
		InviteApiPort:       inviteAPIPort,
		Enabled:             false,
		CreatedAt:           s.clock(),
	}

	if err := s.store.BootstrapNetwork(network, rootCidr, serverPeer); err != nil {
		return nil, fmt.Errorf("bootstrap network: %w", mapStoreError(err))
	}

	return network, nil
}

// DeleteNetwork removes the named network and all of its resources
// (peers, CIDRs, associations, invites, endpoints). The network must
// be stopped before deletion — returns ErrNetworkRunning otherwise.
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

// EnableNetwork persists the enabled flag and starts the network's
// WireGuard devices. Idempotent: if the network is already running
// this is a no-op.
func (s *Service) EnableNetwork(
	ctx context.Context,
	name string,
) error {
	if err := s.store.SetNetworkEnabled(name, true); err != nil {
		return fmt.Errorf("enable network %q: %w", name, mapStoreError(err))
	}
	return s.StartNetwork(ctx, name)
}

// DisableNetwork stops the network's WireGuard devices and persists
// the disabled flag. Idempotent: if the network is already stopped
// this is a no-op.
func (s *Service) DisableNetwork(
	name string,
) error {
	if err := s.StopNetwork(name); err != nil {
		return fmt.Errorf("disable network %q: %w", name, err)
	}
	if err := s.store.SetNetworkEnabled(name, false); err != nil {
		return fmt.Errorf("disable network %q: %w", name, mapStoreError(err))
	}
	return nil
}

// StartNetwork brings up both WireGuard devices (main and invite) for
// the named network and starts the reconciliation loop in the
// background. Non-blocking. Idempotent: starting an already-running
// network is a no-op.
func (s *Service) StartNetwork(
	ctx context.Context,
	name string,
) error {
	s.mu.Lock()
	if _, exists := s.running[name]; exists {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	network, err := s.store.GetNetwork(name)
	if err != nil {
		return fmt.Errorf("start network: %w", mapStoreError(err))
	}

	_, mainNet, err := net.ParseCIDR(network.MainCidr)
	if err != nil {
		return fmt.Errorf("start network: parse main cidr: %w", err)
	}
	mainIfaceAddr := netaddr.InterfaceAddress(mainNet)

	_, inviteNet, err := net.ParseCIDR(network.InviteCidr)
	if err != nil {
		return fmt.Errorf("start network: parse invite cidr: %w", err)
	}
	inviteIfaceAddr := netaddr.InterfaceAddress(inviteNet)

	// Cleanup stack: each successful creation pushes a teardown
	// function. If anything fails during setup, we unwind the
	// whole stack.
	var cleanups []func()
	undo := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	main, err := s.wg.NewDevice(
		network.MainName,
		network.PrivateKey,
		mainIfaceAddr,
		network.MainWireguardPort,
	)
	if err != nil {
		return fmt.Errorf("start network: main device: %w", err)
	}
	cleanups = append(cleanups, func() {
		_ = main.Down()
		_ = s.wg.RemoveDevice(network.MainName)
	})

	if err := main.Up(); err != nil {
		undo()
		return fmt.Errorf("start network: main up: %w", err)
	}

	mainPeers, err := s.buildMainPeers(name)
	if err != nil {
		undo()
		return fmt.Errorf("start network: build main peers: %w", err)
	}
	if err := main.ApplyPeers(mainPeers); err != nil {
		undo()
		return fmt.Errorf("start network: apply main peers: %w", err)
	}

	invite, err := s.wg.NewDevice(
		network.InviteName,
		network.PrivateKey,
		inviteIfaceAddr,
		network.InviteWireguardPort,
	)
	if err != nil {
		undo()
		return fmt.Errorf("start network: invite device: %w", err)
	}
	cleanups = append(cleanups, func() {
		_ = invite.Down()
		_ = s.wg.RemoveDevice(network.InviteName)
	})

	if err := invite.Up(); err != nil {
		undo()
		return fmt.Errorf("start network: invite up: %w", err)
	}

	invitePeers, err := s.buildInvitePeers(name)
	if err != nil {
		undo()
		return fmt.Errorf("start network: build invite peers: %w", err)
	}
	if err := invite.ApplyPeers(invitePeers); err != nil {
		undo()
		return fmt.Errorf("start network: apply invite peers: %w", err)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.running[name] = &NetworkDevices{
		MainName:     network.MainName,
		MainDevice:   main,
		InviteName:   network.InviteName,
		InviteDevice: invite,
		Cancel:       cancel,
	}
	s.mu.Unlock()

	// Start HTTP API listeners if factory is configured.
	if s.apiFactory != nil {
		handlers := s.apiFactory(name)

		// serve main network api
		mainIP := netaddr.FirstAssignable(mainNet)
		mainAddr := netaddr.Endpoint(mainIP, network.MainApiPort)
		mainServer := &http.Server{
			Addr:    mainAddr,
			Handler: handlers.Main,
		}
		go func() {
			if err := mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				s.logf("main HTTP server for %s: %v", name, err)
			}
		}()

		// serve invite network api
		inviteIP := netaddr.FirstAssignable(inviteNet)
		inviteAddr := netaddr.Endpoint(inviteIP, network.InviteApiPort)
		inviteServer := &http.Server{
			Addr:    inviteAddr,
			Handler: handlers.Invite,
		}
		go func() {
			if err := inviteServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				s.logf("invite HTTP server for %s: %v", name, err)
			}
		}()

		s.mu.Lock()
		s.running[name].MainServer = mainServer
		s.running[name].InviteServer = inviteServer
		s.mu.Unlock()
	}

	go s.reconcileLoop(loopCtx, name)

	return nil
}

// StopNetwork brings down both WireGuard devices for the named network
// and stops the reconciliation loop. Idempotent.
func (s *Service) StopNetwork(
	name string,
) error {
	s.mu.Lock()
	devices, exists := s.running[name]
	if !exists {
		s.mu.Unlock()
		return nil
	}
	delete(s.running, name)
	s.mu.Unlock()

	devices.Cancel()

	var errs []error

	// Shut down HTTP servers first (short timeout).
	if devices.MainServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := devices.MainServer.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("main server shutdown: %w", err))
		}
		cancel()
	}
	if devices.InviteServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := devices.InviteServer.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("invite server shutdown: %w", err))
		}
		cancel()
	}

	if err := devices.MainDevice.Down(); err != nil {
		errs = append(errs, fmt.Errorf("main down: %w", err))
	}
	if err := s.wg.RemoveDevice(devices.MainName); err != nil {
		errs = append(errs, fmt.Errorf("remove main: %w", err))
	}

	if err := devices.InviteDevice.Down(); err != nil {
		errs = append(errs, fmt.Errorf("invite down: %w", err))
	}
	if err := s.wg.RemoveDevice(devices.InviteName); err != nil {
		errs = append(errs, fmt.Errorf("remove invite: %w", err))
	}

	return errors.Join(errs...)
}

// defaultInviteCidr is the invite subnet used when CreateNetwork is
// called without an explicit InviteCidr. It lives in the 172.16/12
// private range, which is unlikely to overlap with typical root CIDRs
// in the 10/8 range.
const defaultInviteCidr = "172.16.10.0/24"

// inviteSuffix is the suffix appended to the network name for the invite device
const inviteSuffix = "-i"

// reconcileLoop runs the periodic reconciliation for a started network
// until the context is cancelled.
func (s *Service) reconcileLoop(
	ctx context.Context,
	name string,
) {
	ticker := time.NewTicker(s.reconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileOnce(name)
		}
	}
}

// Reconcile triggers an immediate reconciliation pass for the named
// network, applying the current peer set to both devices. It is the
// exported entry point for the internal reconcileLoop and for
// on-demand reconciliation from admin actions.
func (s *Service) Reconcile(name string) {
	s.reconcileOnce(name)
}

// reconcileOnce performs a single reconciliation pass for a network,
// applying the current peer set to both devices.
func (s *Service) reconcileOnce(
	name string,
) {
	s.mu.Lock()
	devices, exists := s.running[name]
	s.mu.Unlock()
	if !exists {
		return
	}

	mainPeers, err := s.buildMainPeers(name)
	if err != nil {
		s.logf("reconcile %s: build main peers: %v", name, err)
		return
	}
	if err := devices.MainDevice.ApplyPeers(mainPeers); err != nil {
		s.logf("reconcile %s: apply main peers: %v", name, err)
	}

	invitePeers, err := s.buildInvitePeers(name)
	if err != nil {
		s.logf("reconcile %s: build invite peers: %v", name, err)
		return
	}
	if err := devices.InviteDevice.ApplyPeers(invitePeers); err != nil {
		s.logf("reconcile %s: apply invite peers: %v", name, err)
	}
}

// buildMainPeers prunes expired onboarding state, then converts the
// surviving peer list into WireGuard peer configuration for the main
// device. Only enabled peers are included.
func (s *Service) buildMainPeers(
	network string,
) (
	[]wireguard.WGPeer,
	error,
) {
	if err := s.store.PruneExpiredRegistrations(network, s.clock()); err != nil {
		return nil, fmt.Errorf("prune expired registrations: %w", err)
	}

	peers, err := s.store.ListPeers(network)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}

	var wgpeers []wireguard.WGPeer
	for _, peer := range peers {
		if !peer.Enabled {
			continue
		}
		peerCidr, err := netaddr.HostRouteFromCidr(peer.Cidr)
		if err != nil {
			return nil, fmt.Errorf("parse peer CIDR %q: %w", peer.Cidr, err)
		}
		wgpeers = append(wgpeers, wireguard.WGPeer{
			PublicKey:      peer.PublicKey,
			AllowedIPs:     []string{peerCidr.String()},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers, nil
}

// buildInvitePeers prunes expired onboarding state, then converts
// the surviving active registrations into WireGuard peer configuration
// for the invite device.
func (s *Service) buildInvitePeers(
	network string,
) (
	[]wireguard.WGPeer,
	error,
) {
	if err := s.store.PruneExpiredRegistrations(network, s.clock()); err != nil {
		return nil, fmt.Errorf("prune expired registrations: %w", err)
	}

	regs, err := s.store.ListActiveRegistrations(network, s.clock())
	if err != nil {
		return nil, fmt.Errorf("list active registrations: %w", err)
	}

	var wgpeers []wireguard.WGPeer
	for _, reg := range regs {
		route := netaddr.HostRoute(reg.InviteIP)
		wgpeers = append(wgpeers, wireguard.WGPeer{
			PublicKey:      reg.InvitePublicKey,
			AllowedIPs:     []string{route.String()},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers, nil
}

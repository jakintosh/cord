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
	Name             string
	MainName         string // WireGuard interface name for the main device (defaults to Name)
	InviteName       string // WireGuard interface name for the invite device (defaults to Name + "-i")
	PrivateKey       string
	PublicKey        string
	MainCidr         string    // e.g. "10.42.0.0/16" — the network's address space
	InviteCidr       string    // e.g. "10.43.0.0/24" — the invite subnet
	ExternalIP       string    // the server's public IP address
	ListenPort       uint16    // WireGuard listen port for the main interface
	InviteListenPort uint16    // WireGuard listen port for the invite interface
	ApiPort          uint16    // the internal API port served over the tunnel
	Enabled          bool      // whether the network's WireGuard devices are running
	CreatedAt        time.Time // when the network was created
}

// GetNetwork returns the persisted network record by name.
// Returns ErrNotFound if the network does not exist.
func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	nw, err := s.store.GetNetwork(name)
	if err != nil {
		return nil, fmt.Errorf("get network %q: %w", name, mapStoreError(err))
	}
	return nw, nil
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
	cfg Network,
) (
	*Network,
	error,
) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	if cfg.MainName == "" {
		cfg.MainName = cfg.Name
	}
	if cfg.InviteName == "" {
		cfg.InviteName = cfg.Name + inviteSuffix
	}

	if err := wireguard.ValidateDeviceName(cfg.MainName); err != nil {
		return nil, fmt.Errorf("%w: invalid main device name: %v", ErrInvalidInput, err)
	}
	if err := wireguard.ValidateDeviceName(cfg.InviteName); err != nil {
		return nil, fmt.Errorf("%w: invalid invite device name: %v", ErrInvalidInput, err)
	}

	_, mainNet, err := net.ParseCIDR(cfg.MainCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid main CIDR %q: %v", ErrInvalidInput, cfg.MainCidr, err)
	}

	if cfg.ExternalIP == "" {
		return nil, fmt.Errorf("%w: external IP required", ErrInvalidInput)
	}

	if cfg.ListenPort == 0 {
		return nil, fmt.Errorf("%w: listen port required", ErrInvalidInput)
	}

	if cfg.InviteCidr == "" {
		cfg.InviteCidr = defaultInviteCidr
	}

	_, inviteNet, err := net.ParseCIDR(cfg.InviteCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite CIDR %q: %v", ErrInvalidInput, cfg.InviteCidr, err)
	}

	if cfg.InviteListenPort == 0 {
		cfg.InviteListenPort = cfg.ListenPort + 1
	}

	if cfg.ApiPort == 0 {
		cfg.ApiPort = cfg.ListenPort + 2
	}

	if mainNet.Contains(inviteNet.IP) || inviteNet.Contains(mainNet.IP) {
		return nil, fmt.Errorf(
			"%w: invite CIDR %q overlaps main CIDR %q",
			ErrCIDROverlap, cfg.InviteCidr, cfg.MainCidr,
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
		Name:   cfg.Name,
		Cidr:   cfg.MainCidr,
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

	nw := &Network{
		Name:             cfg.Name,
		MainName:         cfg.MainName,
		InviteName:       cfg.InviteName,
		PrivateKey:       privKey,
		PublicKey:        pubKey,
		MainCidr:         cfg.MainCidr,
		InviteCidr:       cfg.InviteCidr,
		ExternalIP:       cfg.ExternalIP,
		ListenPort:       cfg.ListenPort,
		InviteListenPort: cfg.InviteListenPort,
		ApiPort:          cfg.ApiPort,
		Enabled:          false,
		CreatedAt:        s.clock(),
	}

	if err := s.store.BootstrapNetwork(nw, rootCidr, serverPeer); err != nil {
		return nil, fmt.Errorf("bootstrap network: %w", mapStoreError(err))
	}

	return nw, nil
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
	serverIfaceAddr := netaddr.InterfaceAddress(mainNet)

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
		serverIfaceAddr,
		network.ListenPort,
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
		network.InviteListenPort,
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
		Main:       main,
		Invite:     invite,
		MainName:   network.MainName,
		InviteName: network.InviteName,
		Cancel:     cancel,
	}
	s.mu.Unlock()

	// Start HTTP API listeners if factory is configured.
	if s.apiFactory != nil {
		handlers := s.apiFactory(name)

		// serve main network api
		mainIP := netaddr.FirstAssignable(mainNet)
		mainAddr := fmt.Sprintf("%s:%d", mainIP.String(), network.ApiPort)
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
		inviteAddr := fmt.Sprintf("%s:%d", inviteIP.String(), network.ApiPort)
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

	if err := devices.Main.Down(); err != nil {
		errs = append(errs, fmt.Errorf("main down: %w", err))
	}
	if err := s.wg.RemoveDevice(devices.MainName); err != nil {
		errs = append(errs, fmt.Errorf("remove main: %w", err))
	}

	if err := devices.Invite.Down(); err != nil {
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
	if err := devices.Main.ApplyPeers(mainPeers); err != nil {
		s.logf("reconcile %s: apply main peers: %v", name, err)
	}

	invitePeers, err := s.buildInvitePeers(name)
	if err != nil {
		s.logf("reconcile %s: build invite peers: %v", name, err)
		return
	}
	if err := devices.Invite.ApplyPeers(invitePeers); err != nil {
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
	if err := s.store.PruneExpiredInvites(network, s.clock()); err != nil {
		return nil, fmt.Errorf("prune expired invites: %w", err)
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
		wgpeers = append(wgpeers, wireguard.WGPeer{
			PublicKey:      peer.PublicKey,
			AllowedIPs:     []string{peer.Cidr},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers, nil
}

// buildInvitePeers prunes expired onboarding state, then converts
// the surviving active invites into WireGuard peer configuration for
// the invite device.
func (s *Service) buildInvitePeers(
	network string,
) (
	[]wireguard.WGPeer,
	error,
) {
	if err := s.store.PruneExpiredInvites(network, s.clock()); err != nil {
		return nil, fmt.Errorf("prune expired invites: %w", err)
	}

	invites, err := s.store.ListActiveInvites(network, s.clock())
	if err != nil {
		return nil, fmt.Errorf("list active invites: %w", err)
	}

	var wgpeers []wireguard.WGPeer
	for _, invite := range invites {
		route := netaddr.HostRoute(invite.TempIP)
		wgpeers = append(wgpeers, wireguard.WGPeer{
			PublicKey:      invite.TempPubKey,
			AllowedIPs:     []string{route.String()},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers, nil
}

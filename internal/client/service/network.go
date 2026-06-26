package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Network is the persistent record of a client-side network membership.
// It holds the local identity, the assigned address, the server peer
// reference, and the user's enable/disable policy. It is inert domain
// data — the Service owns all behavior.
type Network struct {
	Name           string
	PrivateKey     string
	PublicKey      string
	AssignedCidr   string // e.g. "10.42.0.5/16" — this client's address
	ServerPubkey   string // the server's WireGuard public key
	ServerEndpoint string // server's external endpoint, e.g. "1.2.3.4:51820"
	ServerApiAddr  string // server API through tunnel, e.g. "10.42.0.1:8443"
	Enabled        bool   // whether the daemon should bring up the interface
	CreatedAt      time.Time
}

// NetworkStatus carries the runtime state of a single client network
// for the status endpoint. It combines persisted fields with live
// daemon state.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Running   bool
	Degraded  bool
	LastSync  time.Time
	LastError string
	PeerCount int
}

// Invite carries the parsed contents of a server-issued invite file.
// Fields prefixed with "Temp" describe the invite network; they are used
// only during installation and discarded once the permanent identity is
// assigned.
//
// It is the caller's responsibility to read and parse the invite
// payload from whatever format it arrives in (JSON file, clipboard,
// etc.).
type Invite struct {
	NetworkName    string
	ServerPubkey   string
	ServerEndpoint string // server's external WG endpoint, e.g. "1.2.3.4:51820"
	TempCidr       string // temp address on the invite subnet (step 4)
	TempApiAddr    string // invite API listener, e.g. "10.43.0.1:8443" (step 6 only)
	TempPrivKey    string // invite WG private key (step 4–6)
}

// UpdateNetworkRequest carries the fields that can be changed on an
// installed network. Nil pointer means "no change."
type UpdateNetworkRequest struct {
	Enabled *bool
}

// GetNetwork returns the persisted network record by name.
// Returns ErrNotFound if the network is not installed.
func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.GetNetwork(name)
}

// ListNetworks returns the names of all installed networks.
func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.ListNetworkNames()
}

// ShowNetwork returns the full Network record for a single installed
// network.
func (s *Service) ShowNetwork(
	name string,
) (
	*Network,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.GetNetwork(name)
}

// Status returns the current runtime status for every installed
// network: enabled flag, whether the interface is running, last sync
// time, last error, and peer count.
func (s *Service) Status() (
	[]NetworkStatus,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}

	names, err := s.store.ListNetworkNames()
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	statuses := make([]NetworkStatus, 0, len(names))
	for _, name := range names {
		nw, err := s.store.GetNetwork(name)
		if err != nil {
			continue
		}
		peers, _ := s.store.ListPeers(name)

		status := NetworkStatus{
			Name:      name,
			Enabled:   nw.Enabled,
			PeerCount: len(peers),
		}

		if liveNet, ok := s.running[name]; ok {
			status.Running = true
			status.LastSync = liveNet.LastSync
			status.LastError = liveNet.LastErr
		}

		statuses = append(statuses, status)
	}
	return statuses, nil
}

// InstallNetwork validates an invite, generates a permanent keypair,
// brings up temporary and permanent WireGuard interfaces to communicate
// with the server, redeems the invite, confirms the peer, tears down
// both interfaces, and persists the complete network record.
//
// The new network is left in the disabled state — the caller must call
// EnableNetwork to bring it up. No partial state persists on failure.
func (s *Service) InstallNetwork(
	invite Invite,
) (
	*Network,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	if s.wg == nil {
		return nil, ErrWireGuardUnavailable
	}

	if invite.NetworkName == "" {
		return nil, ErrInvalidInput
	}
	if invite.TempPrivKey == "" {
		return nil, ErrInvalidInput
	}
	if invite.TempCidr == "" {
		return nil, ErrInvalidInput
	}
	if invite.ServerPubkey == "" {
		return nil, ErrInvalidInput
	}
	if invite.ServerEndpoint == "" {
		return nil, ErrInvalidInput
	}
	if invite.TempApiAddr == "" {
		return nil, ErrInvalidInput
	}

	_, err := s.store.GetNetwork(invite.NetworkName)
	if err == nil {
		return nil, ErrNetworkExists
	}

	// Validate device names
	mainName := invite.NetworkName
	if err := wireguard.ValidateDeviceName(mainName); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	inviteName := mainName + "-i"
	if err := wireguard.ValidateDeviceName(inviteName); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	// Generate permanent key pair
	permPrivKey, err := s.wg.GenerateKey()
	if err != nil {
		return nil, err
	}
	permPubKey, err := s.wg.PublicKey(permPrivKey)
	if err != nil {
		return nil, err
	}

	_, inviteCidr, err := net.ParseCIDR(invite.TempCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid temp CIDR %q", ErrInvalidInput, invite.TempCidr)
	}

	// Create invite device
	inviteDev, err := s.wg.NewDevice(inviteName, invite.TempPrivKey, invite.TempCidr, 0)
	if err != nil {
		return nil, fmt.Errorf("create invite device: %w", err)
	}
	defer func() {
		_ = inviteDev.Down()
		_ = s.wg.RemoveDevice(inviteName)
	}()

	// Add invite server peer
	inviteServerPeer := wireguard.WGPeer{
		PublicKey:      invite.ServerPubkey,
		AllowedIPs:     []string{inviteCidr.String()},
		Endpoint:       invite.ServerEndpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	}
	if err := inviteDev.ApplyPeers([]wireguard.WGPeer{inviteServerPeer}); err != nil {
		return nil, fmt.Errorf("apply invite peers: %w", err)
	}

	// Bring up invite device
	if err := inviteDev.Up(); err != nil {
		return nil, fmt.Errorf("bring up invite device: %w", err)
	}
	if err := inviteDev.WaitForHandshake(invite.ServerPubkey, 10*time.Second, nil); err != nil {
		return nil, fmt.Errorf("invite handshake: %w", err)
	}

	// Redeem invite
	tempPubKey, err := s.wg.PublicKey(invite.TempPrivKey)
	if err != nil {
		return nil, err
	}
	inviteAPI := serverapi.NewClient("", invite.TempApiAddr, s.httpClient)
	redeemResult, err := inviteAPI.RedeemInvite(serverapi.RedeemInviteRequest{
		TempPubKey: tempPubKey,
		PermPubKey: permPubKey,
	})
	if err != nil {
		return nil, fmt.Errorf("redeem invite: %w", err)
	}

	// Create main interface
	mainDev, err := s.wg.NewDevice(mainName, permPrivKey, redeemResult.AssignedCidr, 0)
	if err != nil {
		return nil, fmt.Errorf("create main device: %w", err)
	}
	defer func() {
		_ = mainDev.Down()
		_ = s.wg.RemoveDevice(mainName)
	}()

	// Add server peer to main interface
	mainServerPeer := wireguard.WGPeer{
		PublicKey:      invite.ServerPubkey,
		AllowedIPs:     []string{redeemResult.AssignedCidr},
		Endpoint:       invite.ServerEndpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	}
	if err := mainDev.ApplyPeers([]wireguard.WGPeer{mainServerPeer}); err != nil {
		return nil, fmt.Errorf("apply main peers: %w", err)
	}

	// Bring up main interface and wait for handshake
	if err := mainDev.Up(); err != nil {
		return nil, fmt.Errorf("bring up main device: %w", err)
	}
	if err := mainDev.WaitForHandshake(invite.ServerPubkey, 10*time.Second, nil); err != nil {
		return nil, fmt.Errorf("main handshake: %w", err)
	}

	// Confirm peer with server API
	mainAPI := serverapi.NewClient(redeemResult.Server.InternalEndpoint, "", s.httpClient)
	if err := mainAPI.ConfirmPeer(); err != nil {
		return nil, fmt.Errorf("confirm peer: %w", err)
	}

	// Tear down both interfaces
	_ = mainDev.Down()
	_ = s.wg.RemoveDevice(mainName)
	_ = inviteDev.Down()
	_ = s.wg.RemoveDevice(inviteName)

	// Persist the main network
	network := &Network{
		Name:           mainName,
		PrivateKey:     permPrivKey,
		PublicKey:      permPubKey,
		AssignedCidr:   redeemResult.AssignedCidr,
		ServerPubkey:   invite.ServerPubkey,
		ServerEndpoint: invite.ServerEndpoint,
		ServerApiAddr:  redeemResult.Server.InternalEndpoint,
		Enabled:        false,
		CreatedAt:      s.clock(),
	}
	if err := s.store.InsertNetwork(network); err != nil {
		return nil, err
	}

	return network, nil
}

// InsertNetworkDirect persists a pre-built Network record. Exported for
// test seeding.
func (s *Service) InsertNetworkDirect(
	nw *Network,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	return s.store.InsertNetwork(nw)
}

// UninstallNetwork removes a network and all its local state. If the
// network is currently enabled, it is disabled first (interface down,
// sync loop stopped), then the persisted record and peer cache are
// deleted.
func (s *Service) UninstallNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}

	_ = s.DisableNetwork(name)

	return s.store.DeleteNetwork(name)
}

// EnableNetwork brings up the WireGuard interface for the named network
// and starts the background sync loop. It persists enabled=true in the
// store. If any step fails, all partial state is rolled back and the
// store is left with enabled=false.
//
// Idempotent: enabling an already-running network is a no-op.
func (s *Service) EnableNetwork(
	ctx context.Context,
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	if s.wg == nil {
		return ErrWireGuardUnavailable
	}

	s.mu.Lock()
	if _, ok := s.running[name]; ok {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	nw, err := s.store.GetNetwork(name)
	if err != nil {
		return err
	}

	device, err := s.wg.NewDevice(
		nw.Name,
		nw.PrivateKey,
		nw.AssignedCidr,
		0,
	)
	if err != nil {
		return err
	}

	ln := &LiveNetwork{
		Device: device,
	}

	var committed bool
	defer func() {
		if !committed {
			s.stopLive(nw.Name, ln)
			disabled := false
			_, _ = s.store.UpdateNetwork(name, UpdateNetworkRequest{
				Enabled: &disabled,
			})
		}
	}()

	peers, err := s.store.ListPeers(nw.Name)
	if err != nil {
		return err
	}

	wgPeers := s.buildPeers(nw, peers)
	if err := device.ApplyPeers(wgPeers); err != nil {
		return err
	}

	if err := device.Up(); err != nil {
		return err
	}

	s.startLive(nw.Name, ln, nw.ServerApiAddr)

	yes := true
	if _, err := s.store.UpdateNetwork(name, UpdateNetworkRequest{
		Enabled: &yes,
	}); err != nil {
		return err
	}

	committed = true
	return nil
}

// DisableNetwork stops the background sync loop and brings down the
// WireGuard interface for the named network. It persists enabled=false
// in the store.
//
// Idempotent: disabling an already-disabled network is a no-op.
func (s *Service) DisableNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}

	s.mu.Lock()
	ln, ok := s.running[name]
	s.mu.Unlock()

	if ok {
		s.stopLive(name, ln)
	}

	no := false
	_, err := s.store.UpdateNetwork(name, UpdateNetworkRequest{
		Enabled: &no,
	})
	return err
}

// FetchNetwork performs a one-shot peer sync from the server for the
// named network. The network must be running — the server API is only
// reachable through the WireGuard tunnel.
//
// It fetches the visible peer list from the server's peer API,
// reconciles it into the local peer cache, and applies the updated
// peer set to the live WireGuard device.
func (s *Service) FetchNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}

	s.mu.Lock()
	ln, ok := s.running[name]
	s.mu.Unlock()

	if !ok {
		return ErrNetworkNotEnabled
	}

	network, err := s.store.GetNetwork(name)
	if err != nil {
		return err
	}

	peerResponse, err := ln.ApiClient.ListPeers()
	if err != nil {
		return fmt.Errorf("fetch peers for %q: %w", name, err)
	}

	peers := peersFromDTOs(peerResponse)
	if err := s.store.ReconcilePeers(name, peers); err != nil {
		return fmt.Errorf("reconcile peers for %q: %w", name, err)
	}

	updated, err := s.store.ListPeers(name)
	if err != nil {
		return err
	}

	wgPeers := s.buildPeers(network, updated)
	return ln.Device.ApplyPeers(wgPeers)
}

// startLive starts the background sync loop for a device and registers
// the LiveNetwork in s.running. The LiveNetwork must already have a
// valid Device.
func (s *Service) startLive(
	name string,
	liveNet *LiveNetwork,
	apiAddr string,
) {
	ctx := context.Background()
	syncCtx, cancel := context.WithCancel(ctx)
	go s.syncLoop(syncCtx, name)

	liveNet.Cancel = cancel
	liveNet.ApiClient = serverapi.NewClient(apiAddr, "", s.httpClient)

	s.mu.Lock()
	s.running[name] = liveNet
	s.mu.Unlock()
}

// stopLive stops the sync loop, brings the device down, removes the
// device, and unregisters from s.running. Safe to call with a nil
// Cancel — the sync cancellation is skipped when the goroutine was
// never started.
func (s *Service) stopLive(
	name string,
	ln *LiveNetwork,
) {
	if ln.Cancel != nil {
		ln.Cancel()
	}
	_ = ln.Device.Down()
	_ = s.wg.RemoveDevice(name)
	s.mu.Lock()
	delete(s.running, name)
	s.mu.Unlock()
}

// syncLoop is the background goroutine for an enabled network. It runs
// until ctx is cancelled, periodically fetching peers from the server
// and applying changes to the live device. Sync status (LastSync,
// LastErr) is recorded on the LiveNetwork entry.
func (s *Service) syncLoop(
	ctx context.Context,
	name string,
) {
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.doSync(ctx, name)
		}
	}
}

// doSync performs one round of sync for a network and records the
// outcome on the LiveNetwork entry.
func (s *Service) doSync(
	ctx context.Context,
	name string,
) {
	s.mu.Lock()
	ln, ok := s.running[name]
	s.mu.Unlock()
	if !ok {
		return
	}

	err := s.FetchNetwork(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	ln, ok = s.running[name]
	if !ok {
		return
	}

	ln.LastSync = s.clock()
	if err != nil {
		ln.LastErr = err.Error()
	} else {
		ln.LastErr = ""
	}
}

// syncInterval is the period between background peer syncs. Exported
// so tests can assert against it.
const SyncInterval = 30 * time.Second

// buildPeers converts the local peer cache for a network into a slice
// of WGPeer values suitable for ApplyPeers. The server peer is always
// included first with an all-network allowed-ips (for relay); cached
// peers follow with their individual CIDRs.
func (s *Service) buildPeers(
	nw *Network,
	peers []*Peer,
) []wireguard.WGPeer {
	wgPeers := make([]wireguard.WGPeer, 0, len(peers)+1)

	wgPeers = append(wgPeers, wireguard.WGPeer{
		PublicKey:      nw.ServerPubkey,
		AllowedIPs:     []string{nw.AssignedCidr},
		Endpoint:       nw.ServerEndpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	})

	for _, p := range peers {
		wgPeers = append(wgPeers, wireguard.WGPeer{
			PublicKey:      p.PublicKey,
			AllowedIPs:     []string{p.Cidr},
			Endpoint:       p.Endpoint,
			EndpointPolicy: wireguard.EndpointBootstrap,
		})
	}

	return wgPeers
}

// peersFromDTOs converts the server's visible peer list into local Peer
// records. For each peer, the most recent endpoint witness is selected.
func peersFromDTOs(dtos []serverapi.VisiblePeerDTO) []Peer {
	peers := make([]Peer, len(dtos))
	for i, dto := range dtos {
		var endpoint string
		var endpointTime int64
		for _, ep := range dto.Endpoints {
			t := ep.Timestamp.Unix()
			if t > endpointTime {
				endpointTime = t
				endpoint = ep.Endpoint
			}
		}
		peers[i] = Peer{
			Name:         dto.Name,
			PublicKey:    dto.PublicKey,
			Cidr:         dto.Cidr,
			Endpoint:     endpoint,
			EndpointTime: endpointTime,
		}
	}
	return peers
}

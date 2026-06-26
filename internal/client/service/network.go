package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// syncInterval is the period between background peer syncs. Exported
// so tests can assert against it.
const SyncInterval = 30 * time.Second

// ScanInterval is the default period between peer endpoint scans for
// each enabled network.
const ScanInterval = 5 * time.Minute

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
	networkName string,
) (
	*Network,
	error,
) {
	return s.store.GetNetwork(networkName)
}

// ListNetworks returns the names of all installed networks.
func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	return s.store.ListNetworkNames()
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
	network *Network,
) error {
	return s.store.InsertNetwork(network)
}

// UninstallNetwork removes a network and all its local state. If the
// network is currently enabled, it is disabled first (interface down,
// sync loop stopped), then the persisted record and peer cache are
// deleted.
func (s *Service) UninstallNetwork(
	networkName string,
) error {
	_ = s.DisableNetwork(networkName)

	return s.store.DeleteNetwork(networkName)
}

// EnableNetwork brings up the WireGuard interface for the named network
// and starts the background sync loop. It persists enabled=true in the
// store. If any step fails, all partial state is rolled back and the
// store is left with enabled=false.
//
// Idempotent: enabling an already-running network is a no-op.
func (s *Service) EnableNetwork(
	ctx context.Context,
	networkName string,
) error {
	s.mu.Lock()
	if _, ok := s.running[networkName]; ok {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	nw, err := s.store.GetNetwork(networkName)
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
			_, _ = s.store.UpdateNetwork(networkName, UpdateNetworkRequest{
				Enabled: &disabled,
			})
		}
	}()

	if err := s.reconcilePeers(device, nw); err != nil {
		return err
	}

	if err := device.Up(); err != nil {
		return err
	}

	s.startLive(nw.Name, ln, nw.ServerApiAddr)

	yes := true
	if _, err := s.store.UpdateNetwork(networkName, UpdateNetworkRequest{
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
	networkName string,
) error {
	s.mu.Lock()
	ln, ok := s.running[networkName]
	s.mu.Unlock()

	if ok {
		s.stopLive(networkName, ln)
	}

	no := false
	_, err := s.store.UpdateNetwork(networkName, UpdateNetworkRequest{
		Enabled: &no,
	})
	return err
}

// FetchNetwork performs a one-shot peer fetch from the server for the
// named network. The network must be running — the server API is only
// reachable through the WireGuard tunnel.
//
// It fetches the visible peer list from the server's peer API and
// reconciles it into the local peer cache. The cache is not applied
// to the WireGuard device — that is done in the sync loop via reconcilePeers.
func (s *Service) FetchNetwork(
	networkName string,
) error {
	s.mu.Lock()
	ln, ok := s.running[networkName]
	s.mu.Unlock()

	if !ok {
		return ErrNetworkNotEnabled
	}

	peerResponse, err := ln.ApiClient.ListPeers()
	if err != nil {
		return fmt.Errorf("fetch peers for %q: %w", networkName, err)
	}

	peers := peersFromDTOs(peerResponse)
	return s.store.SetPeers(networkName, peers)
}

// Status returns the current runtime status for every installed
// network: enabled flag, whether the interface is running, last sync
// time, last error, and peer count.
func (s *Service) Status() (
	[]NetworkStatus,
	error,
) {
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

// startLive starts the background sync and scan loops for a device
// and registers the LiveNetwork in s.running. The LiveNetwork must
// already have a valid Device.
func (s *Service) startLive(
	name string,
	liveNet *LiveNetwork,
	apiAddr string,
) {
	ctx := context.Background()
	syncCtx, cancel := context.WithCancel(ctx)
	go s.syncLoop(syncCtx, name)
	go s.scanLoop(syncCtx, name)

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
	networkName string,
	ln *LiveNetwork,
) {
	if ln.Cancel != nil {
		ln.Cancel()
	}
	_ = ln.Device.Down()
	_ = s.wg.RemoveDevice(networkName)
	s.mu.Lock()
	delete(s.running, networkName)
	s.mu.Unlock()
}

// syncLoop is the background goroutine for an enabled network. It runs
// until ctx is cancelled, periodically fetching updated peer info from
// the server and syncing the local peer cache to the WireGuard device.
// Sync status (LastSync, LastErr) is recorded on the LiveNetwork entry.
func (s *Service) syncLoop(
	ctx context.Context,
	networkName string,
) {
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx, networkName)
		}
	}
}

// syncOnce performs one round of sync for a network and records the
// outcome on the LiveNetwork entry.
func (s *Service) syncOnce(
	ctx context.Context,
	networkName string,
) {
	s.mu.Lock()
	ln, ok := s.running[networkName]
	s.mu.Unlock()
	if !ok {
		return
	}

	var err error
	if e := s.FetchNetwork(networkName); e != nil {
		err = e
	} else {
		nw, e := s.store.GetNetwork(networkName)
		if e != nil {
			err = e
		} else {
			err = s.reconcilePeers(ln.Device, nw)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ln, ok = s.running[networkName]
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

// scanLoop is the background goroutine that periodically scans the
// live WireGuard device for peer endpoint sightings and reports them
// to the server. Runs until ctx is cancelled.
func (s *Service) scanLoop(
	ctx context.Context,
	networkName string,
) {
	ticker := time.NewTicker(s.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.scanOnce(networkName); err != nil {
				s.logf("scan %s: %v", networkName, err)
			}
		}
	}
}

// scanOnce queries the live WireGuard device for its current peer
// state, reports all known peer endpoints to the server, and updates
// the local cache with any changed endpoints.
func (s *Service) scanOnce(
	networkName string,
) error {
	s.mu.Lock()
	ln, ok := s.running[networkName]
	s.mu.Unlock()
	if !ok {
		return ErrNetworkNotEnabled
	}

	livePeers, err := ln.Device.Status()
	if err != nil {
		return fmt.Errorf("scan %s: status: %w", networkName, err)
	}

	cachedPeers, err := s.store.ListPeers(networkName)
	if err != nil {
		return fmt.Errorf("scan %s: list peers: %w", networkName, err)
	}
	cachedByKey := make(map[string]*Peer, len(cachedPeers))
	for _, p := range cachedPeers {
		cachedByKey[p.PublicKey] = p
	}

	now := s.clock().Unix()
	sightings := make([]serverapi.EndpointSightingDTO, 0, len(livePeers))

	for _, lp := range livePeers {
		cached, inCache := cachedByKey[lp.PublicKey]
		if !inCache || lp.Endpoint == "" {
			continue
		}

		sightings = append(sightings, serverapi.EndpointSightingDTO{
			PeerKey:  lp.PublicKey,
			Endpoint: lp.Endpoint,
		})

		if lp.Endpoint != cached.Endpoint {
			if err := s.store.UpdatePeerEndpoint(
				networkName,
				lp.PublicKey,
				lp.Endpoint,
				now,
			); err != nil {
				s.logf("scan %s: update endpoint for %q: %v", networkName, lp.PublicKey, err)
			}
		}
	}

	if len(sightings) > 0 {
		if err := ln.ApiClient.ReportEndpoints(sightings); err != nil {
			return fmt.Errorf("scan %s: report endpoints: %w", networkName, err)
		}
	}

	return nil
}

// reconcilePeers applies the current peer cache to a WireGuard device.
func (s *Service) reconcilePeers(
	device wireguard.WGDevice,
	network *Network,
) error {
	peers, err := s.store.ListPeers(network.Name)
	if err != nil {
		return err
	}

	wgPeers := make([]wireguard.WGPeer, 0, len(peers)+1)

	serverPeer := wireguard.WGPeer{
		PublicKey:      network.ServerPubkey,
		AllowedIPs:     []string{network.AssignedCidr},
		Endpoint:       network.ServerEndpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	}
	wgPeers = append(wgPeers, serverPeer)

	for _, peer := range peers {
		wgPeers = append(wgPeers, wireguard.WGPeer{
			PublicKey:      peer.PublicKey,
			AllowedIPs:     []string{peer.Cidr},
			Endpoint:       peer.Endpoint,
			EndpointPolicy: wireguard.EndpointBootstrap,
		})
	}

	return device.ApplyPeers(wgPeers)
}

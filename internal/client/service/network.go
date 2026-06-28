package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// SyncInterval is the default period between background peer syncs
// for each enabled network.
const SyncInterval = 30 * time.Second

// ScanInterval is the default period between endpoint sighting
// reports to the server for each enabled network.
const ScanInterval = 5 * time.Minute

// StaleThreshold is the duration after which a peer with no handshake
// is considered unhealthy and eligible for endpoint rotation.
const StaleThreshold = 90 * time.Second

// DefaultBackoff is the base duration for exponential backoff when a
// degraded peer has exhausted all candidate endpoints.
const DefaultBackoff = 5 * time.Minute

// MaxBackoff caps the exponential backoff duration.
const MaxBackoff = 1 * time.Hour

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
	// Validate invite
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

	// Check if network already exists
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

	inviteIfaceAddr, err := netaddr.ParseInterface(invite.TempCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid temp CIDR %q", ErrInvalidInput, invite.TempCidr)
	}

	// Create invite device
	inviteDev, err := s.wg.NewDevice(inviteName, invite.TempPrivKey, inviteIfaceAddr, 0)
	if err != nil {
		return nil, fmt.Errorf("create invite device: %w", err)
	}
	cleanupInvite := func() {
		if inviteDev == nil {
			return
		}
		_ = inviteDev.Down()
		_ = s.wg.RemoveDevice(inviteName)
		inviteDev = nil
	}
	defer cleanupInvite()

	// Bring up invite device
	if err := inviteDev.Up(); err != nil {
		return nil, fmt.Errorf("bring up invite device: %w", err)
	}

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

	// Redeem invite
	inviteAPI := serverapi.NewClient("", invite.TempApiAddr, s.httpClient)
	redeemResult, err := inviteAPI.RedeemInvite(serverapi.RedeemInviteRequest{
		PermPubKey: permPubKey,
	})
	if err != nil {
		return nil, fmt.Errorf("redeem invite: %w", err)
	}

	cleanupInvite()

	mainIfaceAddr, err := netaddr.ParseInterface(redeemResult.AssignedCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid assigned CIDR %q", ErrInvalidInput, redeemResult.AssignedCidr)
	}

	// Create main interface
	mainDev, err := s.wg.NewDevice(mainName, permPrivKey, mainIfaceAddr, 0)
	if err != nil {
		return nil, fmt.Errorf("create main device: %w", err)
	}
	cleanupMain := func() {
		if mainDev == nil {
			return
		}
		_ = mainDev.Down()
		_ = s.wg.RemoveDevice(mainName)
		mainDev = nil
	}
	defer cleanupMain()

	// Bring up main interface
	if err := mainDev.Up(); err != nil {
		return nil, fmt.Errorf("bring up main device: %w", err)
	}

	mainAllowedIP, err := networkRoute(redeemResult.AssignedCidr)
	if err != nil {
		return nil, fmt.Errorf("main server route: %w", err)
	}

	// Add server peer to main interface
	mainServerPeer := wireguard.WGPeer{
		PublicKey:      invite.ServerPubkey,
		AllowedIPs:     []string{mainAllowedIP},
		Endpoint:       redeemResult.Server.ExternalEndpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	}
	if err := mainDev.ApplyPeers([]wireguard.WGPeer{mainServerPeer}); err != nil {
		return nil, fmt.Errorf("apply main peers: %w", err)
	}

	// Confirm peer with server API
	mainAPI := serverapi.NewClient(redeemResult.Server.InternalEndpoint, "", s.httpClient)
	if err := mainAPI.ConfirmPeer(); err != nil {
		return nil, fmt.Errorf("confirm peer: %w", err)
	}

	cleanupMain()

	// Persist the main network
	network := &Network{
		Name:           mainName,
		PrivateKey:     permPrivKey,
		PublicKey:      permPubKey,
		AssignedCidr:   redeemResult.AssignedCidr,
		ServerPubkey:   invite.ServerPubkey,
		ServerEndpoint: redeemResult.Server.ExternalEndpoint,
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
// and starts the background network loop. It persists enabled=true in
// the store. If any step fails, all partial state is rolled back and
// the store is left with enabled=false.
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

	ifaceAddr, err := netaddr.ParseInterface(nw.AssignedCidr)
	if err != nil {
		return fmt.Errorf("%w: invalid assigned CIDR %q", ErrInvalidInput, nw.AssignedCidr)
	}

	device, err := s.wg.NewDevice(nw.Name, nw.PrivateKey, ifaceAddr, 0)
	if err != nil {
		return err
	}

	liveNet := &LiveNetwork{
		Device:       device,
		ServerPubkey: nw.ServerPubkey,
		Degraded:     make(map[string]*DegradedPeer),
	}

	var committed bool
	defer func() {
		if !committed {
			s.stopLive(nw.Name, liveNet)
			_ = s.store.SetNetworkEnabled(networkName, false)
		}
	}()

	if err := device.Up(); err != nil {
		return err
	}

	if err := s.reconcilePeers(device, nw); err != nil {
		return err
	}

	s.startLive(nw.Name, liveNet, nw.ServerApiAddr)

	if err := s.store.SetNetworkEnabled(networkName, true); err != nil {
		return err
	}

	committed = true
	return nil
}

// DisableNetwork stops the background loop and brings down the
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

	return s.store.SetNetworkEnabled(networkName, false)
}

// FetchNetwork performs a one-shot peer fetch from the server for the
// named network. The network must be running — the server API is only
// reachable through the WireGuard tunnel.
//
// It fetches the visible peer list from the server's peer API and
// reconciles it into the local peer cache and endpoint catalog.
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
	if err := s.store.SetPeers(networkName, peers); err != nil {
		return fmt.Errorf("set peers for %q: %w", networkName, err)
	}

	// Populate the endpoint catalog from server gossip.
	for _, dto := range peerResponse {
		eps := endpointsFromDTO(dto)
		if len(eps) > 0 {
			if err := s.store.SetPeerEndpoints(networkName, dto.PublicKey, eps); err != nil {
				s.logf("fetch %s: set endpoints for %q: %v", networkName, dto.PublicKey, err)
			}
		}
	}

	return nil
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
			status.Degraded = len(liveNet.Degraded) > 0
		}

		statuses = append(statuses, status)
	}
	return statuses, nil
}

// startLive starts the background network loop for a device and
// registers the LiveNetwork in s.running. The LiveNetwork must already
// have a valid Device.
func (s *Service) startLive(
	name string,
	liveNet *LiveNetwork,
	apiAddr string,
) {
	// start the network loop in the background
	ctx := context.Background()
	loopCtx, cancel := context.WithCancel(ctx)
	go s.networkLoop(loopCtx, name)

	// configure the livenet
	liveNet.Cancel = cancel
	liveNet.ApiClient = serverapi.NewClient(apiAddr, "", s.httpClient)

	s.mu.Lock()
	s.running[name] = liveNet
	s.mu.Unlock()
}

// stopLive stops the network loop, brings the device down, removes the
// device, and unregisters from s.running. Safe to call with a nil
// Cancel — cancellation is skipped when the goroutine was never started.
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

// networkLoop is the single background goroutine for an enabled network.
// It runs until ctx is cancelled, executing three sequential blocks on
// each tick: sync (gated), scan+tick (always), and report (gated).
func (s *Service) networkLoop(
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
			s.networkTick(networkName)
		}
	}
}

// networkTick performs one iteration of the network loop: sync,
// scan+tick, and report.
func (s *Service) networkTick(
	networkName string,
) {
	s.mu.Lock()
	ln, ok := s.running[networkName]
	s.mu.Unlock()
	if !ok {
		return
	}

	now := s.clock()

	// --- Sync block (gated by syncInterval) ---
	if now.Sub(ln.LastSync) >= s.syncInterval {
		s.syncBlock(networkName, ln)
	}

	// --- Scan+tick block (every tick) ---
	sightings := s.scanAndTickBlock(networkName, ln, now)

	// --- Report block (gated by scanInterval) ---
	if now.Sub(ln.LastScan) >= s.scanInterval && len(sightings) > 0 {
		if err := ln.ApiClient.ReportEndpoints(sightings); err != nil {
			s.logf("report %s: %v", networkName, err)
		}
		ln.LastScan = now
	}
}

// syncBlock fetches the peer list and endpoint catalog from the server,
// reconciles the peer cache with WireGuard, and updates degraded peer
// candidate lists with any new endpoints.
func (s *Service) syncBlock(
	networkName string,
	ln *LiveNetwork,
) {
	var err error
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		ln, ok := s.running[networkName]
		if !ok {
			return
		}
		ln.LastSync = s.clock()
		if err != nil {
			ln.LastErr = err.Error()
		} else {
			ln.LastErr = ""
		}
	}()

	// Fetch peers and endpoints from server.
	err = s.FetchNetwork(networkName)
	if err != nil {
		return
	}

	// Reconcile peers to WireGuard.
	nw, e := s.store.GetNetwork(networkName)
	if e != nil {
		err = e
		return
	}
	err = s.reconcilePeers(ln.Device, nw)
	if err != nil {
		return
	}

	// Reconcile degraded peer candidate lists with fresh endpoint data.
	s.reconcileDegraded(networkName, ln)
}

// reconcileDegraded updates the candidate lists for all degraded peers
// using the latest endpoint data from the store. It culls removed
// endpoints, appends new ones, and wakes idle peers if fresh candidates
// are available.
func (s *Service) reconcileDegraded(
	networkName string,
	ln *LiveNetwork,
) {
	for pubKey, peer := range ln.Degraded {
		fresh, err := s.store.ListPeerEndpoints(networkName, pubKey)
		if err != nil {
			s.logf("sync %s: list endpoints for degraded %q: %v", networkName, pubKey, err)
			continue
		}

		freshSet := make(map[string]struct{}, len(fresh))
		for _, ep := range fresh {
			freshSet[ep.Endpoint] = struct{}{}
		}

		// Cull: keep only candidates still in the fresh set.
		// Track how many are removed before the current index.
		newCandidates := make([]string, 0, len(peer.Candidates))
		removedBefore := 0
		for i, c := range peer.Candidates {
			if _, ok := freshSet[c]; ok {
				newCandidates = append(newCandidates, c)
			} else if i < peer.Index {
				removedBefore++
			}
		}
		peer.Candidates = newCandidates
		peer.Index -= removedBefore
		if peer.Index > len(peer.Candidates) {
			peer.Index = len(peer.Candidates)
		}
		if peer.Index < 0 {
			peer.Index = 0
		}

		// Append: add fresh endpoints not already in candidate list.
		existing := make(map[string]struct{}, len(peer.Candidates))
		for _, c := range peer.Candidates {
			existing[c] = struct{}{}
		}
		for _, ep := range fresh {
			if _, ok := existing[ep.Endpoint]; !ok {
				peer.Candidates = append(peer.Candidates, ep.Endpoint)
			}
		}

		// Wake if there are untried candidates.
		if peer.Index < len(peer.Candidates) {
			peer.Idle = false
		}
	}
}

// scanAndTickBlock reads the live WireGuard state, classifies peers as
// healthy or degraded, records local endpoint observations, and ticks
// the degraded peer state machines. Returns any endpoint sightings to
// report.
func (s *Service) scanAndTickBlock(
	networkName string,
	ln *LiveNetwork,
	now time.Time,
) []serverapi.EndpointSightingDTO {
	livePeers, err := ln.Device.Status()
	if err != nil {
		s.logf("scan %s: status: %v", networkName, err)
		return nil
	}

	nowUnix := now.Unix()
	sightings := make([]serverapi.EndpointSightingDTO, 0, len(livePeers))

	// Track which degraded peers are still live and unhealthy.
	activeDegraded := make(map[string]struct{})

	for _, lp := range livePeers {
		if lp.PublicKey == ln.ServerPubkey {
			continue
		}

		healthy := !lp.LastHandshake.IsZero() &&
			now.Sub(lp.LastHandshake) < StaleThreshold

		if healthy {
			// Peer is healthy — remove any degraded state.
			delete(ln.Degraded, lp.PublicKey)

			// Record local observation.
			if lp.Endpoint != "" {
				if err := s.store.UpdatePeerEndpointLocal(
					networkName, lp.PublicKey, lp.Endpoint, nowUnix,
				); err != nil {
					s.logf("scan %s: update local endpoint for %q: %v",
						networkName, lp.PublicKey, err)
				}
				sightings = append(sightings, serverapi.EndpointSightingDTO{
					PeerKey:  lp.PublicKey,
					Endpoint: lp.Endpoint,
				})
			}
			continue
		}

		// Peer is unhealthy.
		activeDegraded[lp.PublicKey] = struct{}{}

		// Create degraded state if not already tracked.
		if _, ok := ln.Degraded[lp.PublicKey]; !ok {
			endpoints, e := s.store.ListPeerEndpoints(networkName, lp.PublicKey)
			if e != nil {
				s.logf("scan %s: list endpoints for %q: %v",
					networkName, lp.PublicKey, e)
				continue
			}
			candidates := make([]string, len(endpoints))
			for i, ep := range endpoints {
				candidates[i] = ep.Endpoint
			}
			ln.Degraded[lp.PublicKey] = &DegradedPeer{
				Candidates:  candidates,
				Idle:        len(candidates) == 0,
				NextAttempt: now,
			}
		}
	}

	// Prune degraded entries for peers no longer live or now healthy.
	for pubKey := range ln.Degraded {
		if _, active := activeDegraded[pubKey]; !active {
			delete(ln.Degraded, pubKey)
		}
	}

	// Tick all degraded peer state machines.
	for pubKey, peer := range ln.Degraded {
		s.tickDegraded(networkName, ln, pubKey, peer, now)
	}

	return sightings
}

// tickDegraded advances the state machine for a single degraded peer.
// It handles idle backoff, cycle resets, and endpoint rotation.
func (s *Service) tickDegraded(
	networkName string,
	ln *LiveNetwork,
	pubKey string,
	peer *DegradedPeer,
	now time.Time,
) {
	// Skip idle peers whose backoff hasn't expired.
	if peer.Idle && now.Before(peer.NextAttempt) {
		return
	}

	// If idle but backoff expired, reset the cycle.
	if peer.Idle {
		endpoints, err := s.store.ListPeerEndpoints(networkName, pubKey)
		if err != nil {
			s.logf("tick %s: list endpoints for %q: %v", networkName, pubKey, err)
			return
		}
		peer.Candidates = make([]string, len(endpoints))
		for i, ep := range endpoints {
			peer.Candidates[i] = ep.Endpoint
		}
		peer.Index = 0
		peer.Idle = false
	}

	// If no candidates, go idle with backoff.
	if len(peer.Candidates) == 0 {
		peer.Idle = true
		peer.LoopCount++
		peer.NextAttempt = now.Add(degradedBackoff(peer.LoopCount))
		return
	}

	// Rotate to next candidate.
	endpoint := peer.Candidates[peer.Index]
	if err := ln.Device.UpdateEndpoint(pubKey, endpoint); err != nil {
		s.logf("tick %s: update endpoint for %q to %q: %v",
			networkName, pubKey, endpoint, err)
	}
	peer.Index++

	// If exhausted all candidates, go idle with backoff.
	if peer.Index >= len(peer.Candidates) {
		peer.Idle = true
		peer.LoopCount++
		peer.NextAttempt = now.Add(degradedBackoff(peer.LoopCount))
	}
}

// degradedBackoff returns the exponential backoff duration for the
// given loop count, capped at MaxBackoff.
func degradedBackoff(loopCount int) time.Duration {
	if loopCount <= 0 {
		return DefaultBackoff
	}
	d := DefaultBackoff * (1 << (loopCount - 1))
	if d > MaxBackoff {
		d = MaxBackoff
	}
	return d
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

	serverRoute, err := networkRoute(network.AssignedCidr)
	if err != nil {
		return fmt.Errorf("server route: %w", err)
	}

	wgPeers := make([]wireguard.WGPeer, 0, len(peers)+1)

	serverPeer := wireguard.WGPeer{
		PublicKey:      network.ServerPubkey,
		AllowedIPs:     []string{serverRoute},
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

func networkRoute(
	cidr string,
) (
	string,
	error,
) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", cidr, err)
	}
	return network.String(), nil
}

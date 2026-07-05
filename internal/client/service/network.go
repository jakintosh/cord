package service

import (
	"fmt"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const (
	// SyncInterval is the default period between full peer-set reconciles
	// for each enabled network.
	SyncInterval = 2 * time.Minute

	// ScanInterval governs how often the loop reads live handshake state,
	// classifies peers as healthy or degraded, and checks for due rotations.
	ScanInterval = 30 * time.Second

	// ReportInterval governs how often accumulated endpoint sightings are
	// flushed to the server.
	ReportInterval = 5 * time.Minute

	// StaleThreshold is the duration after which a peer with no handshake
	// is considered unhealthy and eligible for endpoint rotation.
	StaleThreshold = 90 * time.Second
)

// NetworkConfig is the permanent membership record. Complete at insert,
// immutable except Enabled.
type NetworkConfig struct {
	Name          string
	PrivateKey    string
	InterfaceName string
	AssignedRoute string
	Server        ServerInfo
	Enabled       bool
	CreatedAt     time.Time
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

// --- Runtime Network ---

// Network is the runtime owner of one enabled network. It owns a Tunnel,
// a peer API client, and a background goroutine that drives sync, scan,
// and report. The loop goroutine owns all mutable state; only the
// status fields are shared, guarded by statusMu.
type Network struct {
	cfg    NetworkConfig
	tunnel *Tunnel
	store  Store
	client *serverapi.PeerClient
	clock  func() time.Time
	logf   func(string, ...any)

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	statusMu      sync.Mutex
	lastSync      time.Time
	lastErr       string
	degradedCount int

	degraded  map[string]*degradedPeer
	sightings []serverapi.EndpointSightingDTO

	stopCh  chan struct{}
	syncReq chan chan error
	doneCh  chan struct{}
}

// InsertNetworkDirect persists a pre-built NetworkConfig record.
// Exported for test seeding only.
func (s *Service) InsertNetworkDirect(
	nc *NetworkConfig,
) error {
	return s.store.InsertNetwork(nc)
}

// EnableNetwork brings up the WireGuard interface for the named
// network and starts the background loop. It persists enabled=true.
// Idempotent: enabling an already-running network is a no-op.
func (s *Service) EnableNetwork(
	networkName string,
) error {
	s.mu.Lock()
	if _, ok := s.running[networkName]; ok {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	nc, err := s.store.GetNetwork(networkName)
	if err != nil {
		return err
	}

	tunnel, err := newTunnel(
		s.wireguard,
		nc.InterfaceName,
		nc.PrivateKey,
		nc.AssignedRoute,
		nc.Server,
	)
	if err != nil {
		return err
	}

	var committed bool
	defer func() {
		if !committed {
			_ = tunnel.stop()
			_ = s.store.SetNetworkEnabled(networkName, false)
		}
	}()

	client := s.newPeerClient(tunnel)
	network := s.newNetwork(nc, tunnel, client)

	if err := network.start(); err != nil {
		return err
	}

	if err := s.store.SetNetworkEnabled(networkName, true); err != nil {
		return err
	}

	s.mu.Lock()
	s.running[networkName] = network
	s.mu.Unlock()

	committed = true
	return nil
}

// DisableNetwork stops the background loop and brings down the
// WireGuard interface for the named network. It persists enabled=false.
// Idempotent: disabling an already-disabled network is a no-op.
func (s *Service) DisableNetwork(
	networkName string,
) error {
	s.mu.Lock()
	n, ok := s.running[networkName]
	if ok {
		delete(s.running, networkName)
	}
	s.mu.Unlock()

	if ok {
		if err := n.stop(); err != nil {
			s.logf("disable: stop network %q: %v", networkName, err)
		}
	}

	return s.store.SetNetworkEnabled(networkName, false)
}

// Sync triggers an on-demand peer fetch and device reconciliation
// for the named running network. Returns ErrNetworkNotEnabled if the
// network is not running.
func (s *Service) Sync(
	networkName string,
) error {
	s.mu.Lock()
	n, ok := s.running[networkName]
	s.mu.Unlock()

	if !ok {
		return ErrNetworkNotEnabled
	}

	return n.requestSync()
}

// UninstallNetwork removes a network and all its local state. If the
// network is currently enabled, it is disabled first. If a mid-install
// Install record exists but no network, the install row is deleted.
func (s *Service) UninstallNetwork(
	networkName string,
) error {
	_ = s.DisableNetwork(networkName)

	if err := s.store.DeleteNetwork(networkName); err == nil {
		return nil
	}

	return s.store.DeleteInstall(networkName)
}

// GetNetworkStatus returns the runtime status for a single named network.
func (s *Service) GetNetworkStatus(
	networkName string,
) (
	NetworkStatus,
	error,
) {
	nc, err := s.store.GetNetwork(networkName)
	if err != nil {
		return NetworkStatus{}, err
	}

	peers, _ := s.store.ListPeers(networkName)

	status := NetworkStatus{
		Name:      networkName,
		Enabled:   nc.Enabled,
		PeerCount: len(peers),
	}

	s.mu.Lock()
	n, ok := s.running[networkName]
	s.mu.Unlock()

	if ok {
		lastSync, lastErr, degradedCount := n.snapshot()
		status.Running = true
		status.LastSync = lastSync
		status.LastError = lastErr
		status.Degraded = degradedCount > 0
	}

	return status, nil
}

// ListNetworkStatuses returns the runtime status for every installed
// network.
func (s *Service) ListNetworkStatuses() (
	[]NetworkStatus,
	error,
) {
	names, err := s.store.ListNetworkNames()
	if err != nil {
		return nil, err
	}

	statuses := make([]NetworkStatus, 0, len(names))
	for _, name := range names {
		status, err := s.GetNetworkStatus(name)
		if err != nil {
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// GetNetwork returns the persisted network config by name.
func (s *Service) GetNetwork(
	networkName string,
) (
	*NetworkConfig,
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

// newNetwork builds a runtime Network fully wired for its loop: the
// degraded map and all loop channels are initialized so the struct is
// never half-built.
func (s *Service) newNetwork(
	nc *NetworkConfig,
	tunnel *Tunnel,
	client *serverapi.PeerClient,
) *Network {
	return &Network{
		cfg:            *nc,
		tunnel:         tunnel,
		store:          s.store,
		client:         client,
		clock:          s.clock,
		logf:           s.logf,
		syncInterval:   s.syncInterval,
		scanInterval:   s.scanInterval,
		reportInterval: s.reportInterval,
		degraded:       make(map[string]*degradedPeer),
		stopCh:         make(chan struct{}),
		syncReq:        make(chan chan error),
		doneCh:         make(chan struct{}),
	}
}

// snapshot returns the last sync time, last error string, and current
// degraded peer count under the status lock.
func (n *Network) snapshot() (
	time.Time,
	string,
	int,
) {
	n.statusMu.Lock()
	defer n.statusMu.Unlock()
	return n.lastSync, n.lastErr, n.degradedCount
}

// start reconciles the device from the local peer cache synchronously,
// then launches the background loop. Applying the cached peer set is
// local-only and must happen at enable time; the first server sync
// happens immediately in the loop.
func (n *Network) start() error {
	if err := n.reconcilePeers(); err != nil {
		return err
	}
	go n.loop()
	return nil
}

// stop signals the loop, waits for it to exit, then stops the tunnel.
// Called exactly once per Network — removal from the Service registry
// is the ownership transfer that guarantees it.
func (n *Network) stop() error {
	close(n.stopCh)
	<-n.doneCh
	return n.tunnel.stop()
}

// requestSync sends a sync request to the loop goroutine and blocks
// until it completes.
func (n *Network) requestSync() error {
	reply := make(chan error, 1)
	select {
	case n.syncReq <- reply:
		return <-reply
	case <-n.doneCh:
		return ErrNetworkNotEnabled
	}
}

// loop is the single background goroutine for the network.
func (n *Network) loop() {
	defer close(n.doneCh)

	if err := n.sync(); err != nil {
		n.logf("sync %s: %v", n.cfg.Name, err)
	}

	syncTicker := time.NewTicker(n.syncInterval)
	scanTicker := time.NewTicker(n.scanInterval)
	reportTicker := time.NewTicker(n.reportInterval)
	defer syncTicker.Stop()
	defer scanTicker.Stop()
	defer reportTicker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-syncTicker.C:
			if err := n.sync(); err != nil {
				n.logf("sync %s: %v", n.cfg.Name, err)
			}
		case <-scanTicker.C:
			n.scan(n.clock())
		case <-reportTicker.C:
			n.report()
		case replyCh := <-n.syncReq:
			replyCh <- n.sync()
		}
	}
}

// sync is the ONLY writer of the device peer set. It fetches the
// visible peer list from the server, persists the cache, reconciles
// the device, and refreshes degraded candidates. The last sync time and
// error are recorded for status readers.
func (n *Network) sync() (err error) {
	defer func() {
		n.statusMu.Lock()
		n.lastSync = n.clock()
		if err != nil {
			n.lastErr = err.Error()
		} else {
			n.lastErr = ""
		}
		n.statusMu.Unlock()
	}()

	peerDtos, err := n.client.ListPeers()
	if err != nil {
		return fmt.Errorf("fetch peers: %w", err)
	}

	peers := peersFromDTOs(peerDtos)
	if err := n.store.SetPeers(n.cfg.Name, peers); err != nil {
		return fmt.Errorf("set peers: %w", err)
	}

	for _, dto := range peerDtos {
		eps := endpointsFromDTO(dto)
		if len(eps) > 0 {
			if err := n.store.SetPeerEndpoints(n.cfg.Name, dto.PublicKey, eps); err != nil {
				n.logf("sync %s: set endpoints for %q: %v", n.cfg.Name, dto.PublicKey, err)
			}
		}
	}

	if err := n.reconcilePeers(); err != nil {
		return err
	}

	n.refreshDegraded()
	return nil
}

// scan reads the live WireGuard state, classifies peers as healthy or
// degraded, records local endpoint observations, and attempts due
// rotations.
func (n *Network) scan(now time.Time) {
	devicePeers, err := n.tunnel.device.Peers()
	if err != nil {
		n.logf("scan %s: peers: %v", n.cfg.Name, err)
		return
	}

	nowUnix := now.Unix()

	activeDegraded := make(map[string]struct{})

	for _, lp := range devicePeers {
		pubKey := lp.PublicKey.String()
		if pubKey == n.cfg.Server.PublicKey {
			continue
		}

		healthy := !lp.LastHandshake.IsZero() &&
			now.Sub(lp.LastHandshake) < StaleThreshold

		if healthy {
			delete(n.degraded, pubKey)

			if lp.Endpoint != nil {
				endpoint := lp.Endpoint.String()
				if err := n.store.UpdatePeerEndpointLocal(
					n.cfg.Name, pubKey, endpoint, nowUnix,
				); err != nil {
					n.logf("scan %s: update local endpoint for %q: %v",
						n.cfg.Name, pubKey, err)
				}
				n.sightings = append(n.sightings, serverapi.EndpointSightingDTO{
					PeerKey:  pubKey,
					Endpoint: endpoint,
				})
			}
			continue
		}

		activeDegraded[pubKey] = struct{}{}

		if _, ok := n.degraded[pubKey]; !ok {
			endpoints, e := n.store.ListPeerEndpoints(n.cfg.Name, pubKey)
			if e != nil {
				n.logf("scan %s: list endpoints for %q: %v",
					n.cfg.Name, pubKey, e)
				continue
			}
			candidates := make([]string, len(endpoints))
			for i, ep := range endpoints {
				candidates[i] = ep.Endpoint
			}
			n.degraded[pubKey] = newDegradedPeer(candidates, now)
		}
	}

	for pubKey := range n.degraded {
		if _, active := activeDegraded[pubKey]; !active {
			delete(n.degraded, pubKey)
		}
	}

	for pubKey, dp := range n.degraded {
		endpoint, ok := dp.rotate(now)
		if !ok {
			continue
		}
		if err := n.tunnel.device.SetPeerEndpoint(pubKey, endpoint); err != nil {
			n.logf("scan %s: update endpoint for %q to %q: %v",
				n.cfg.Name, pubKey, endpoint, err)
		}
	}

	n.statusMu.Lock()
	n.degradedCount = len(n.degraded)
	n.statusMu.Unlock()
}

// report flushes accumulated endpoint sightings to the server.
func (n *Network) report() {
	if len(n.sightings) == 0 {
		return
	}
	if err := n.client.ReportEndpoints(n.sightings); err != nil {
		n.logf("report %s: %v", n.cfg.Name, err)
	}
	n.sightings = nil
}

// refreshDegraded updates the candidate lists for all degraded peers
// using the latest endpoint data from the store.
func (n *Network) refreshDegraded() {
	for pubKey, dp := range n.degraded {
		fresh, err := n.store.ListPeerEndpoints(n.cfg.Name, pubKey)
		if err != nil {
			n.logf("sync %s: list endpoints for degraded %q: %v", n.cfg.Name, pubKey, err)
			continue
		}
		candidates := make([]string, len(fresh))
		for i, ep := range fresh {
			candidates[i] = ep.Endpoint
		}
		dp.refresh(candidates)
	}
}

// reconcilePeers applies the current peer cache to the WireGuard device.
func (n *Network) reconcilePeers() error {
	peers, err := n.store.ListPeers(n.cfg.Name)
	if err != nil {
		return err
	}

	wgPeers := make([]wireguard.PeerConfig, 0, len(peers)+1)

	wgPeers = append(wgPeers, wireguard.PeerConfig{
		PublicKey:      n.cfg.Server.PublicKey,
		AllowedIPs:     []string{n.cfg.Server.Route},
		Endpoint:       n.cfg.Server.Endpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	})

	for _, peer := range peers {
		peerRoute, err := netaddr.ParseRoute(peer.Route)
		if err != nil {
			return fmt.Errorf("parse peer route %q: %w", peer.Route, err)
		}
		wgPeers = append(wgPeers, wireguard.PeerConfig{
			PublicKey:      peer.PublicKey,
			AllowedIPs:     []string{peerRoute.String()},
			Endpoint:       peer.Endpoint,
			EndpointPolicy: wireguard.EndpointBootstrap,
		})
	}

	return n.tunnel.device.SetPeers(wgPeers...)
}

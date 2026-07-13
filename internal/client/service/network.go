package service

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const (
	// SyncInterval is the default period between full peer-set syncs
	// from the server. Syncs are 1:N — every client polls the same
	// server — so this stays coarse.
	SyncInterval = 2 * time.Minute

	// ScanInterval governs how often live handshake state is read
	// from the local device. Scans are purely local, so this can be
	// frequent.
	ScanInterval = 30 * time.Second

	// ReportInterval governs how often locally observed endpoints are
	// sent to the server. Reports are 1:N, so this stays coarse.
	ReportInterval = 5 * time.Minute

	// StaleThreshold is the duration after which a peer with no
	// handshake is considered stale and eligible for endpoint
	// rotation.
	StaleThreshold = wireguard.ActiveHandshakeThreshold

	// EndpointTTL bounds the local endpoint catalog. A candidate remains while
	// it has been observed by either this client or the server within this
	// window.
	EndpointTTL = 24 * time.Hour
)

// NetworkOptions controls local settings for a network installation.
type NetworkOptions struct {
	// ListenPort is the local WireGuard UDP port. Nil uses the service default.
	ListenPort *uint16
}

// NetworkConfig is the permanent membership record. Complete at insert,
// with local settings and Enabled mutable after installation.
type NetworkConfig struct {
	Name          string
	PrivateKey    string
	InterfaceName string
	AssignedRoute string
	ListenPort    uint16
	Server        ServerInfo
	Enabled       bool
	CreatedAt     time.Time
}

// Network is a running client network: one Tunnel plus three
// self-rearming activity timers. All durable state lives in the store;
// the activities (sync, scan, report) are projections between the
// store, the device, and the server.
type Network struct {
	cfg    NetworkConfig
	tunnel *Tunnel
	store  Store
	client *client.PeerClient
	clock  func() time.Time
	log    *slog.Logger

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	mu      sync.Mutex // one activity at a time
	stopped bool

	syncTimer   *time.Timer
	scanTimer   *time.Timer
	reportTimer *time.Timer
}

// IsNetworkRunning reports whether the named network is currently
// running.
func (s *Service) IsNetworkRunning(
	name string,
) bool {
	s.mu.Lock()
	_, ok := s.running[name]
	s.mu.Unlock()
	return ok
}

// GetNetwork returns the persisted network config by name.
func (s *Service) GetNetwork(
	name string,
) (
	*NetworkConfig,
	error,
) {
	return s.store.GetNetwork(name)
}

// ListNetworks returns the names of all installed networks.
func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	return s.store.ListNetworkNames()
}

// InsertNetworkDirect persists a pre-built NetworkConfig record.
// Exported for test seeding only.
func (s *Service) InsertNetworkDirect(
	cfg *NetworkConfig,
) error {
	return s.store.InsertNetwork(cfg)
}

// EnableNetwork brings up the WireGuard interface for the named
// network and starts the activity timers. It persists enabled=true.
// Idempotent: enabling an already-running network is a no-op.
func (s *Service) EnableNetwork(
	name string,
) error {
	s.mu.Lock()
	if _, ok := s.running[name]; ok {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	cfg, err := s.store.GetNetwork(name)
	if err != nil {
		return err
	}

	tunnel, err := newTunnel(
		s.wireguard,
		cfg.InterfaceName,
		cfg.PrivateKey,
		cfg.AssignedRoute,
		cfg.Server,
		cfg.ListenPort,
	)
	if err != nil {
		return err
	}

	var committed bool
	defer func() {
		if !committed {
			_ = tunnel.stop()
			_ = s.store.SetNetworkEnabled(name, false)
		}
	}()

	client, err := s.newPeerClient(tunnel)
	if err != nil {
		return fmt.Errorf("create peer client: %w", err)
	}

	network := s.newNetwork(cfg, tunnel, client)
	if err := network.start(); err != nil {
		return err
	}

	if err := s.store.SetNetworkEnabled(name, true); err != nil {
		return err
	}

	s.mu.Lock()
	s.running[name] = network
	s.mu.Unlock()

	committed = true
	s.log.Info("network enabled", "network", name, "interface", cfg.InterfaceName)
	return nil
}

// DisableNetwork stops the activity timers and brings down the
// WireGuard interface for the named network. It persists enabled=false.
// Idempotent: disabling an already-disabled network is a no-op.
func (s *Service) DisableNetwork(
	name string,
) error {
	s.mu.Lock()
	n, ok := s.running[name]
	if ok {
		delete(s.running, name)
	}
	s.mu.Unlock()

	if ok {
		if err := n.stop(); err != nil {
			s.log.Warn("disable: stop network failed", "network", name, "err", err)
		}
		s.log.Info("network disabled", "network", name)
	}

	return s.store.SetNetworkEnabled(name, false)
}

// UpdateNetwork persists local network configuration. When a running
// network's listen port changes, the network is restarted so it takes effect.
func (s *Service) UpdateNetwork(
	name string,
	opts NetworkOptions,
) error {
	if _, err := s.store.GetNetwork(name); err != nil {
		return err
	}
	if opts.ListenPort == nil {
		return ErrInvalidInput
	}

	running := s.IsNetworkRunning(name)
	if running {
		if err := s.DisableNetwork(name); err != nil {
			return err
		}
	}

	if err := s.store.UpdateNetwork(name, opts); err != nil {
		if running {
			if restartErr := s.EnableNetwork(name); restartErr != nil {
				return errors.Join(err, fmt.Errorf("restore running network: %w", restartErr))
			}
		}
		return err
	}

	if running {
		if err := s.EnableNetwork(name); err != nil {
			return fmt.Errorf("restart network with updated configuration: %w", err)
		}
	}
	return nil
}

// SyncNetwork triggers an on-demand peer fetch and device reconciliation
// for the named running network. Returns ErrNetworkNotEnabled if the
// network is not running.
func (s *Service) SyncNetwork(
	name string,
) error {
	s.mu.Lock()
	n, ok := s.running[name]
	s.mu.Unlock()

	if !ok {
		return ErrNetworkNotEnabled
	}

	return n.sync()
}

// newNetwork builds a runtime Network. The activity timers are armed
// by start.
func (s *Service) newNetwork(
	cfg *NetworkConfig,
	tunnel *Tunnel,
	client *client.PeerClient,
) *Network {
	return &Network{
		cfg:            *cfg,
		tunnel:         tunnel,
		store:          s.store,
		client:         client,
		clock:          s.clock,
		log:            s.log.With("network", cfg.Name),
		syncInterval:   s.syncInterval,
		scanInterval:   s.scanInterval,
		reportInterval: s.reportInterval,
	}
}

// start reconciles the device from the local peer cache, then arms the
// activity timers. Applying the cached peer set is local-only: it
// works offline, and its failure aborts the enable synchronously —
// the first server sync, firing immediately on its own timer, is a
// freshness upgrade that can only be logged. The lock is held so the
// immediate sync cannot run against a partially armed timer set.
//
// The timers report to no caller, so errors are captured in logging
// closures here.
func (n *Network) start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if err := n.reconcile(); err != nil {
		return err
	}

	n.syncTimer = time.AfterFunc(0, func() {
		if err := n.sync(); err != nil {
			n.log.Warn("sync failed", "err", err)
		}
	})
	n.scanTimer = time.AfterFunc(n.scanInterval, func() {
		if err := n.scan(); err != nil {
			n.log.Warn("scan failed", "err", err)
		}
	})
	n.reportTimer = time.AfterFunc(n.reportInterval, func() {
		if err := n.report(); err != nil {
			n.log.Warn("report failed", "err", err)
		}
	})
	return nil
}

// stop halts the activity timers and closes the tunnel. An activity
// already in flight finishes first; one already waiting on the lock
// sees stopped and returns without touching the device.
func (n *Network) stop() error {
	n.mu.Lock()
	n.stopped = true
	n.syncTimer.Stop()
	n.scanTimer.Stop()
	n.reportTimer.Stop()
	n.mu.Unlock()

	return n.tunnel.stop()
}

// sync fetches the visible peer list from the server, persists it,
// projects it onto the device, and schedules the next sync. It is the
// only writer of the full device peer set. Called by both the sync
// timer and on-demand Service.Sync, so an on-demand sync defers the
// next scheduled one.
func (n *Network) sync() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.stopped {
		return ErrNetworkNotEnabled
	}
	defer n.syncTimer.Reset(n.syncInterval)

	visible, err := n.client.ListPeers()
	if err != nil {
		return fmt.Errorf("fetch peers: %w", err)
	}
	n.log.Debug("sync", "peers", len(visible))

	peers := peersFromProtocol(visible)
	if err := n.store.SetPeers(n.cfg.Name, peers); err != nil {
		return fmt.Errorf("set peers: %w", err)
	}

	for _, vp := range visible {
		eps := endpointsFromProtocol(vp)
		if len(eps) == 0 {
			continue
		}
		if err := n.store.SetPeerEndpoints(n.cfg.Name, vp.PublicKey, eps); err != nil {
			n.log.Warn("sync: set endpoints failed", "peer", vp.PublicKey, "err", err)
		}
	}

	if err := n.store.DeletePeerEndpointsBefore(
		n.cfg.Name,
		n.clock().Add(-EndpointTTL).Unix(),
	); err != nil {
		return fmt.Errorf("prune endpoints: %w", err)
	}

	return n.reconcile()
}

// scan reads live handshake state from the device and schedules the
// next scan. Healthy peers get their current endpoint recorded as
// locally observed; stale peers get their next candidate endpoint
// applied.
func (n *Network) scan() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.stopped {
		return nil
	}
	defer n.scanTimer.Reset(n.scanInterval)

	now := n.clock()

	devicePeers, err := n.tunnel.device.Peers()
	if err != nil {
		return fmt.Errorf("peers: %w", err)
	}

	for _, peer := range devicePeers {
		pubKey := peer.PublicKey.String()
		if pubKey == n.cfg.Server.PublicKey {
			continue
		}

		healthy := !peer.LastHandshake.IsZero() &&
			now.Sub(peer.LastHandshake) < StaleThreshold

		if !healthy {
			n.rotate(pubKey, now)
			continue
		}

		if peer.Endpoint == nil {
			continue
		}

		if err := n.store.UpdatePeerEndpointLocal(
			n.cfg.Name, pubKey, peer.Endpoint.String(), now.Unix(),
		); err != nil {
			n.log.Warn("scan: record endpoint failed", "peer", pubKey, "err", err)
		}
	}

	return nil
}

// report sends endpoints observed locally within the last report
// window to the server for gossip distribution, and schedules the
// next report.
func (n *Network) report() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.stopped {
		return nil
	}
	defer n.reportTimer.Reset(n.reportInterval)

	since := n.clock().Add(-n.reportInterval).Unix()
	sightings, err := n.store.ListLocalEndpointsSince(n.cfg.Name, since)
	if err != nil {
		return fmt.Errorf("list local endpoints: %w", err)
	}
	if len(sightings) == 0 {
		return nil
	}

	// Convert the stored domain sightings into the protocol wire shape
	// at this single network boundary. Storage stays wire-agnostic; the
	// report path owns the transformation.
	n.log.Debug("report", "sightings", len(sightings))
	reports := sightingsToProtocol(sightings)
	return n.client.ReportEndpoints(reports)
}

// reconcile applies the current peer cache to the WireGuard device.
func (n *Network) reconcile() error {
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

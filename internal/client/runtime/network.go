package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const (
	// SyncInterval is the default period between full network snapshot
	// syncs from the server. Syncs are 1:N — every client polls the same
	// server — so this stays coarse.
	SyncInterval = 2 * time.Minute

	// ScanInterval governs how often live handshake state is read from
	// the local device. Scans are purely local, so this can be frequent.
	ScanInterval = 30 * time.Second

	// ReportInterval governs how often locally observed endpoints are
	// sent to the server. Reports are 1:N, so this stays coarse.
	ReportInterval = 5 * time.Minute

	// PersistentKeepaliveInterval keeps client-side peer endpoints reachable
	// through NAT and gives WireGuard traffic with which to probe bootstrap and
	// rotated endpoints.
	PersistentKeepaliveInterval = 25 * time.Second

	// StaleThreshold is the duration after which a peer with no
	// handshake is considered stale and eligible for endpoint rotation.
	StaleThreshold = wireguard.ActiveHandshakeThreshold
)

// Network is a client network running in this process: one Tunnel plus
// three self-rearming activity timers. It holds no durable state of its
// own — every read and write goes through the service, and the
// activities (sync, scan, report) are projections between the store, the
// device, and the server.
type Network struct {
	record  *service.Network
	tunnel  *Tunnel
	service *service.Service
	client  *client.PeerClient
	clock   func() time.Time
	log     *slog.Logger

	// ctx is the network's slice of the runtime context. Its
	// cancellation is what retires the activity timers, so none of them
	// can outlive the device it configures.
	ctx    context.Context
	cancel context.CancelFunc

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	// mu is held for the whole of an activity, so stop() waits for one
	// in flight and no activity survives the stop.
	mu              sync.Mutex
	syncTimer       *time.Timer
	scanTimer       *time.Timer
	reportTimer     *time.Timer
	reconcileStatus ActivityStatus
	syncStatus      ActivityStatus
	scanStatus      ActivityStatus
	reportStatus    ActivityStatus
}

// start brings up the tunnel under ctx, reconciles the device from the
// local peer cache, then arms the activity timers. Applying the cached
// peer set is local-only: it works offline, and its failure aborts the
// start synchronously — the first server sync, firing immediately on its
// own timer, is a freshness upgrade that can only be logged. The lock is
// held so that sync cannot run against a partially armed timer set.
//
// The timers report to no caller, so errors are captured in logging
// closures here.
func (n *Network) start(
	ctx context.Context,
	wg *wireguard.Manager,
	httpClient *http.Client,
) error {
	n.ctx, n.cancel = context.WithCancel(ctx)

	n.tunnel = &Tunnel{
		name:       n.record.InterfaceName,
		privateKey: n.record.PrivateKey,
		route:      n.record.AssignedRoute,
		listenPort: n.record.ListenPort,
		server:     n.record.Server,
		keepalive:  PersistentKeepaliveInterval,
	}
	if err := n.tunnel.start(wg); err != nil {
		n.cancel()
		return err
	}

	peerNetClient, err := client.NewPeerClient(n.tunnel.apiAddr, httpClient)
	if err != nil {
		n.cancel()
		if stopErr := n.tunnel.stop(); stopErr != nil {
			n.log.Debug("stop tunnel after failed start", "err", stopErr)
		}
		return fmt.Errorf("create peer client: %w", err)
	}
	n.client = peerNetClient

	n.mu.Lock()
	defer n.mu.Unlock()

	if err := n.applyPeers(); err != nil {
		n.cancel()
		if stopErr := n.tunnel.stop(); stopErr != nil {
			n.log.Debug("stop tunnel after failed start", "err", stopErr)
		}
		return err
	}

	n.reconcileStatus.record(n.clock(), nil)

	// Sync now: the cached peer set may be stale, so freshen it first.
	n.syncTimer = time.AfterFunc(0, func() {
		if err := n.sync(); err != nil {
			n.log.Warn("sync failed", "err", err)
		}
	})

	// No immediate scan: no handshakes have accumulated yet, so peers
	// would all look stale and get needlessly rotated.
	n.scanTimer = time.AfterFunc(n.scanInterval, func() {
		if err := n.scan(); err != nil {
			n.log.Warn("scan failed", "err", err)
		}
	})

	// No immediate report: nothing has been observed locally yet.
	n.reportTimer = time.AfterFunc(n.reportInterval, func() {
		if err := n.report(); err != nil {
			n.log.Warn("report failed", "err", err)
		}
	})

	return nil
}

// stop cancels the network context, retires the activity timers, waits
// for any activity in flight, and closes the tunnel. An activity already
// waiting on the lock sees the cancelled context and returns without
// touching the device.
func (n *Network) stop() error {
	n.cancel()

	n.mu.Lock()
	n.syncTimer.Stop()
	n.scanTimer.Stop()
	n.reportTimer.Stop()
	n.mu.Unlock()

	return n.tunnel.stop()
}

// reconcile applies the current peer cache to the device. It is the
// converge entry point for a network that is already where it should be.
func (n *Network) reconcile() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isStopped() {
		return nil
	}

	err := n.applyPeers()
	n.reconcileStatus.record(n.clock(), err)
	return err
}

// sync fetches the visible network snapshot from the server, hands it to
// the service to validate and persist, projects the resulting peers onto
// the device, and schedules the next sync. Called by both the sync timer
// and on-demand Runtime.Sync, so an on-demand sync defers the next
// scheduled one.
func (n *Network) sync() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isStopped() {
		return service.ErrNetworkNotEnabled
	}

	refreshSync := func(err error) error {
		n.completeRefresh(
			n.syncTimer,
			n.syncInterval,
			&n.syncStatus,
			err,
		)
		return err
	}

	snapshot, err := n.client.GetSnapshot(n.ctx)
	if err != nil {
		return refreshSync(fmt.Errorf("fetch network snapshot: %w", err))
	}

	n.log.Debug(
		"sync",
		"peers",
		len(snapshot.Peers),
		"topology_nodes",
		len(snapshot.Topology.Nodes),
	)

	if err := n.service.ApplyNetworkSnapshot(
		n.record.Name,
		snapshot,
	); err != nil {
		return refreshSync(err)
	}

	if err := n.applyPeers(); err != nil {
		return refreshSync(err)
	}

	return refreshSync(nil)
}

// scan reads live handshake state from the device and schedules the
// next scan. Healthy peers get their current endpoint recorded as
// locally observed; stale peers get their next candidate endpoint
// applied.
func (n *Network) scan() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isStopped() {
		return nil
	}

	refreshScan := func(err error) error {
		n.completeRefresh(
			n.scanTimer,
			n.scanInterval,
			&n.scanStatus,
			err,
		)
		return err
	}

	now := n.clock()

	devicePeerStatuses, err := n.tunnel.device.Peers()
	if err != nil {
		return refreshScan(fmt.Errorf("peers: %w", err))
	}

	var errs []error
	for _, peerStatus := range devicePeerStatuses {
		pubKey := peerStatus.PublicKey.String()
		if pubKey == n.record.Server.PublicKey {
			continue
		}

		if !peerHealthy(peerStatus, now) {
			if err := n.rotate(pubKey, now); err != nil {
				errs = append(errs, err)
			}
			continue
		}

		if peerStatus.Endpoint == nil {
			continue
		}

		if err := n.service.RecordLocalEndpoint(
			n.record.Name,
			pubKey,
			peerStatus.Endpoint.String(),
			now,
		); err != nil {
			errs = append(errs, fmt.Errorf("record endpoint for peer %q: %w", pubKey, err))

			n.log.Warn(
				"scan: record endpoint failed",
				"peer",
				pubKey,
				"err",
				err,
			)
		}
	}

	return refreshScan(errors.Join(errs...))
}

// report sends endpoints observed locally within the last report
// window to the server for gossip distribution, and schedules the
// next report.
func (n *Network) report() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isStopped() {
		return nil
	}

	refreshReport := func(err error) error {
		n.completeRefresh(
			n.reportTimer,
			n.reportInterval,
			&n.reportStatus,
			err,
		)
		return err
	}

	since := n.clock().Add(-n.reportInterval)
	sightings, err := n.service.ListLocalEndpoints(n.record.Name, since)
	if err != nil {
		return refreshReport(fmt.Errorf("list local endpoints: %w", err))
	}
	if len(sightings) == 0 {
		return refreshReport(nil)
	}

	n.log.Debug(
		"report",
		"sightings",
		len(sightings),
	)

	sightingsDTO := sightingsToProtocol(sightings)
	if err := n.client.ReportEndpoints(n.ctx, sightingsDTO); err != nil {
		return refreshReport(fmt.Errorf("report endpoints: %w", err))
	}

	return refreshReport(nil)
}

// applyPeers projects the cached peer set onto the WireGuard device.
// Callers hold n.mu.
func (n *Network) applyPeers() error {
	peers, err := n.service.ListPeers(n.record.Name)
	if err != nil {
		return err
	}

	wgPeers := make([]wireguard.PeerConfig, 0, len(peers)+1)

	// always manually add the server peer first
	wgPeers = append(wgPeers, wireguard.PeerConfig{
		PublicKey:           n.record.Server.PublicKey,
		AllowedIPs:          []string{n.record.Server.Route},
		Endpoint:            n.record.Server.Endpoint,
		EndpointPolicy:      wireguard.EndpointFixed,
		PersistentKeepalive: int(PersistentKeepaliveInterval / time.Second),
	})

	for _, peer := range peers {
		peerRoute, err := netaddr.ParseRoute(peer.Route)
		if err != nil {
			return fmt.Errorf("parse peer route %q: %w", peer.Route, err)
		}
		wgPeers = append(wgPeers, wireguard.PeerConfig{
			PublicKey:           peer.PublicKey,
			AllowedIPs:          []string{peerRoute.String()},
			Endpoint:            peer.Endpoint,
			EndpointPolicy:      wireguard.EndpointBootstrap,
			PersistentKeepalive: int(PersistentKeepaliveInterval / time.Second),
		})
	}

	return n.tunnel.device.SetPeers(wgPeers...)
}

// completeRefresh records the completion of an activity attempt and
// rearms its timer. It is called with n.mu held, including on error
// paths. A stopped network rearms nothing.
func (n *Network) completeRefresh(
	timer *time.Timer,
	cadence time.Duration,
	status *ActivityStatus,
	err error,
) {
	status.record(n.clock(), err)
	if n.isStopped() {
		return
	}
	timer.Reset(cadence)
}

// isStopped reports whether this network has been isStopped.
func (n *Network) isStopped() bool {
	return n.ctx.Err() != nil
}

// isDiverged reports whether the stored record describes a different
// tunnel than the one this network is running. A isDiverged network is
// restarted rather than reconciled.
func (n *Network) isDiverged(
	record *service.Network,
) bool {
	return n.record.InterfaceName != record.InterfaceName ||
		n.record.PrivateKey != record.PrivateKey ||
		n.record.AssignedRoute != record.AssignedRoute ||
		n.record.ListenPort != record.ListenPort ||
		n.record.Server != record.Server
}

// getActivityStatuses returns a consistent snapshot of this network's work.
func (n *Network) getActivityStatuses() (
	ActivityStatus,
	ActivityStatus,
	ActivityStatus,
	ActivityStatus,
) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.reconcileStatus, n.syncStatus, n.scanStatus, n.reportStatus
}

// getPeerStatuses reports the live device state for this network's getPeerStatuses.
func (n *Network) getPeerStatuses() (
	[]wireguard.PeerStatus,
	error,
) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isStopped() {
		return nil, nil
	}
	return n.tunnel.device.Peers()
}

// peerHealthy reports whether a peer's last handshake is recent enough
// to consider it connected. Peers that have never handshaken are not
// healthy.
func peerHealthy(
	peerStatus wireguard.PeerStatus,
	now time.Time,
) bool {
	return !peerStatus.LastHandshake.IsZero() &&
		now.Sub(peerStatus.LastHandshake) < StaleThreshold
}

// sightingsToProtocol converts stored domain sightings into the protocol
// wire shape. Storage stays wire-agnostic; the report path owns the
// transformation at this single network boundary.
func sightingsToProtocol(
	sightings []service.EndpointSighting,
) []protocol.EndpointSighting {
	reports := make([]protocol.EndpointSighting, len(sightings))
	for i, sighting := range sightings {
		reports[i] = protocol.EndpointSighting{
			PeerKey:  sighting.PeerKey,
			Endpoint: sighting.Endpoint,
		}
	}
	return reports
}

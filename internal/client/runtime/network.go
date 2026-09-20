package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
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

type networkOptions struct {
	record     *service.Network
	service    *service.Service
	wireguard  *wireguard.Manager
	dns        DNSManager
	httpClient *http.Client
	clock      func() time.Time
	log        *slog.Logger

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration
}

// Network is a configured client network owned by the runtime loop.
// It owns a tunnel, a DNS resolver, and activity due times. Durable state is read and
// written through the service. Its activities (sync, scan, report) project
// between the store, the device, and the server.
type Network struct {
	record  *service.Network
	tunnel  *Tunnel
	service *service.Service
	dns     *dnsResolver
	client  *client.PeerClient
	clock   func() time.Time
	log     *slog.Logger

	// ctx follows the runtime lifetime and is canceled when this network stops.
	ctx    context.Context
	cancel context.CancelFunc
	//
	// Only the runtime loop accesses closing. Once cleanup
	// begins, this instance must be retired before a replacement can start.
	closing bool

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	reconcileStatus ActivityStatus
	cleanupStatus   ActivityStatus
	syncStatus      ActivityStatus
	scanStatus      ActivityStatus
	reportStatus    ActivityStatus
}

// newNetwork constructs the tunnel, DNS resolver, and API client without starting resources
// or scheduling activities. Runtime supplies resolved dependencies and intervals.
func newNetwork(
	opts networkOptions,
) (
	*Network,
	error,
) {
	tunnel, err := newTunnel(tunnelOptions{
		name:       opts.record.InterfaceName,
		privateKey: opts.record.PrivateKey,
		route:      opts.record.AssignedRoute,
		listenPort: opts.record.ListenPort,
		server:     opts.record.Server,
	}, opts.wireguard)
	if err != nil {
		return nil, err
	}

	peerNetClient, err := client.NewPeerClient(tunnel.apiAddr, opts.httpClient)
	if err != nil {
		return nil, fmt.Errorf("create peer client: %w", err)
	}

	serverRoute, err := netaddr.ParseRoute(opts.record.Server.Route)
	if err != nil {
		return nil, fmt.Errorf("DNS server route: %w", err)
	}

	return &Network{
		record: opts.record,
		tunnel: tunnel,
		dns: &dnsResolver{
			manager: opts.dns,
			device:  tunnel.device,
			config: dns.Config{
				Network:    opts.record.Name,
				Domain:     opts.record.Name + ".internal",
				Nameserver: serverRoute.IP,
			},
			clock: opts.clock,
		},
		service:        opts.service,
		client:         peerNetClient,
		clock:          opts.clock,
		log:            opts.log,
		syncInterval:   opts.syncInterval,
		scanInterval:   opts.scanInterval,
		reportInterval: opts.reportInterval,
		syncStatus:     ActivityStatus{Interval: opts.syncInterval},
		scanStatus:     ActivityStatus{Interval: opts.scanInterval},
		reportStatus:   ActivityStatus{Interval: opts.reportInterval},
	}, nil
}

// start reconciles the tunnel and cached peers before scheduling activities.
// Startup works offline; the immediate server sync only freshens the cache.
// The runtime owns cleanup if startup fails.
func (n *Network) start(
	ctx context.Context,
) error {
	n.ctx, n.cancel = context.WithCancel(ctx)

	if err := n.reconcile(); err != nil {
		n.cancel()
		return err
	}

	now := n.clock()
	n.syncStatus.NextAttemptAt = now

	// Give handshakes and local observations time to accumulate.
	n.scanStatus.NextAttemptAt = now.Add(n.scanInterval)
	n.reportStatus.NextAttemptAt = now.Add(n.reportInterval)

	return nil
}

// reconcile repairs the tunnel before applying the current peer cache.
func (n *Network) reconcile() (err error) {
	if n.isStopped() {
		return nil
	}

	var dnsErr error
	defer func() {
		n.reconcileStatus.record(n.clock(), errors.Join(err, dnsErr))
	}()

	if err := n.tunnel.reconcile(n.ctx); err != nil {
		return err
	}

	if err := n.applyPeers(); err != nil {
		return err
	}

	dnsErr = n.dns.reconcile(n.ctx)
	if dnsErr != nil {
		// DNS failure degrades the network without rolling back its tunnel or
		// preventing sync, scan, and report from being scheduled at startup.
		n.log.Warn(
			"DNS reconciliation failed",
			"err", dnsErr,
		)
	}

	return nil
}

// stop cancels the network lifetime and closes its tunnel.
func (n *Network) stop() (
	err error,
) {
	defer func() {
		n.cleanupStatus.record(n.clock(), err)
	}()

	if n.cancel != nil {
		n.cancel()
	}

	n.syncStatus.NextAttemptAt = time.Time{}
	n.scanStatus.NextAttemptAt = time.Time{}
	n.reportStatus.NextAttemptAt = time.Time{}

	// Cleanup must remain usable after network or daemon cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), dnsCleanupTimeout)
	defer cancel()

	dnsErr := n.dns.close(ctx)
	tunnelErr := n.tunnel.close()

	return errors.Join(dnsErr, tunnelErr)
}

// nextActivity returns the earliest due time, or zero for a retired network.
func (n *Network) nextActivity() time.Time {
	if n.closing || n.isStopped() {
		return time.Time{}
	}

	var next time.Time
	for _, due := range []time.Time{
		n.syncStatus.NextAttemptAt,
		n.scanStatus.NextAttemptAt,
		n.reportStatus.NextAttemptAt,
	} {
		if due.IsZero() {
			continue
		}
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return next
}

// runDueActivities runs on the runtime goroutine. Each activity schedules its
// next attempt from completion, so overdue work runs once without catching up.
func (n *Network) runDueActivities() {
	if n.closing || n.isStopped() {
		return
	}
	if !n.clock().Before(n.syncStatus.NextAttemptAt) {
		if err := n.sync(n.ctx); err != nil {
			n.log.Warn("sync failed", "err", err)
		}
	}

	if n.isStopped() {
		return
	}
	if !n.clock().Before(n.scanStatus.NextAttemptAt) {
		if err := n.scan(); err != nil {
			n.log.Warn("scan failed", "err", err)
		}
	}

	if n.isStopped() {
		return
	}
	if !n.clock().Before(n.reportStatus.NextAttemptAt) {
		if err := n.report(); err != nil {
			n.log.Warn("report failed", "err", err)
		}
	}
}

// sync fetches the visible network snapshot from the server, hands it to
// the service to validate and persist, projects the resulting peers onto
// the device, and schedules the next sync. Called by both scheduled work
// and on-demand Runtime.Sync, so an on-demand sync defers the next
// scheduled one.
func (n *Network) sync(
	ctx context.Context,
) (err error) {
	if n.isStopped() {
		return service.ErrNetworkNotEnabled
	}

	defer func() {
		n.completeRefresh(n.syncInterval, &n.syncStatus, err)
	}()

	snapshot, err := n.client.GetSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("fetch network snapshot: %w", err)
	}

	n.log.Debug(
		"sync",
		"peers", len(snapshot.Peers),
		"topology_nodes", len(snapshot.Topology.Nodes),
	)

	if err := n.service.ApplyNetworkSnapshot(
		n.record.Name,
		snapshot,
	); err != nil {
		return err
	}

	if err := n.applyPeers(); err != nil {
		return err
	}

	return nil
}

// scan reads live handshake state from the device and schedules the
// next scan. Healthy peers get their current endpoint recorded as
// locally observed; stale peers get their next candidate endpoint
// applied.
func (n *Network) scan() (err error) {
	if n.isStopped() {
		return nil
	}

	defer func() {
		n.completeRefresh(n.scanInterval, &n.scanStatus, err)
	}()

	now := n.clock()

	devicePeerStatuses, err := n.tunnel.device.GetPeerStatuses()
	if err != nil {
		return fmt.Errorf("peers: %w", err)
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

	return errors.Join(errs...)
}

// report sends endpoints observed locally within the last report
// window to the server for gossip distribution, and schedules the
// next report.
func (n *Network) report() (err error) {
	if n.isStopped() {
		return nil
	}

	defer func() {
		n.completeRefresh(n.reportInterval, &n.reportStatus, err)
	}()

	since := n.clock().Add(-n.reportInterval)
	sightings, err := n.service.ListLocalEndpoints(n.record.Name, since)
	if err != nil {
		return fmt.Errorf("list local endpoints: %w", err)
	}
	if len(sightings) == 0 {
		return nil
	}

	n.log.Debug("report", "sightings", len(sightings))

	sightingsDTO := sightingsToProtocol(sightings)
	if err := n.client.ReportEndpoints(n.ctx, sightingsDTO); err != nil {
		return fmt.Errorf("report endpoints: %w", err)
	}

	return nil
}

// applyPeers projects the cached peer set onto the WireGuard device.
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

// completeRefresh records an attempt and schedules the next one after completion.
// Failed attempts use the same cadence. A stopped network schedules nothing.
func (n *Network) completeRefresh(
	cadence time.Duration,
	status *ActivityStatus,
	err error,
) {
	now := n.clock()
	status.record(now, err)
	if n.isStopped() {
		return
	}
	status.NextAttemptAt = now.Add(cadence)
}

// isStopped reports whether activities have no active network lifetime.
func (n *Network) isStopped() bool {
	return n.ctx == nil || n.ctx.Err() != nil
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
	return n.reconcileStatus, n.syncStatus, n.scanStatus, n.reportStatus
}

// getPeerStatuses reports the live device state for this network's getPeerStatuses.
func (n *Network) getPeerStatuses() (
	[]wireguard.PeerStatus,
	error,
) {
	if n.isStopped() {
		return nil, nil
	}

	return n.tunnel.device.GetPeerStatuses()
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

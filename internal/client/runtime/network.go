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
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
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

const serverPeerName = "cord-server"

// NetworkLookup contains exactly one record on a successful lookup.
type NetworkLookup struct {
	Network *service.Network
	Install *service.Install
}

// NetworkTopology is the network's cached topology joined with live peer
// connectivity for display. Connected maps a peer name to its live
// state; peers absent from the map have no known connectivity. The
// subject peer is always present: its connectivity is whether the
// network is running. The pinned server peer is evaluated from its
// live WireGuard handshake.
type NetworkTopology struct {
	View        topology.View
	GeneratedAt time.Time
	SyncedAt    time.Time
	Connected   map[string]bool
}

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

	reconcileStatus resource.ActivityStatus
	cleanupStatus   resource.ActivityStatus
	syncStatus      resource.ActivityStatus
	scanStatus      resource.ActivityStatus
	reportStatus    resource.ActivityStatus
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
		syncStatus:     resource.ActivityStatus{Interval: opts.syncInterval},
		scanStatus:     resource.ActivityStatus{Interval: opts.scanInterval},
		reportStatus:   resource.ActivityStatus{Interval: opts.reportInterval},
	}, nil
}

// status collects resource health and activity without changing the network.
func (n *Network) status() resource.Snapshot {
	target := resource.Running
	if n.closing {
		target = resource.Stopped
	}

	state := resource.Running
	if n.isStopped() {
		state = resource.Stopped
	}
	if n.cleanupStatus.Error != "" {
		state = resource.Failed
	}

	status := resource.Snapshot{
		Kind:   "network",
		Name:   n.record.Name,
		State:  state,
		Target: target,
		Activities: map[string]resource.ActivityStatus{
			"reconcile": n.reconcileStatus,
			"sync":      n.syncStatus,
			"scan":      n.scanStatus,
			"report":    n.reportStatus,
			"close":     n.cleanupStatus,
		},
		Children: []resource.Snapshot{
			n.tunnel.status(target),
			n.dns.status(target),
		},
	}
	status.UpdateHealth()
	return status
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
		n.reconcileStatus.Record(n.clock(), errors.Join(err, dnsErr))
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
		n.cleanupStatus.Record(n.clock(), err)
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
	status *resource.ActivityStatus,
	err error,
) {
	now := n.clock()
	status.Record(now, err)
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

// ListNetworks returns installed network records, ordered by name.
func (r *Runtime) ListNetworks(
	ctx context.Context,
) (
	[]*service.Network,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Network, error) {
		return r.service.ListNetworks()
	})
}

// LookupNetwork resolves an installed network or a pending installation in one
// command, so confirmation cannot occur between the two reads.
func (r *Runtime) LookupNetwork(
	ctx context.Context,
	name string,
) (
	NetworkLookup,
	error,
) {
	return executeValue(r, ctx, func() (NetworkLookup, error) {
		network, err := r.service.GetNetwork(name)
		if err == nil {
			return NetworkLookup{Network: network}, nil
		}
		if !errors.Is(err, service.ErrNotFound) {
			return NetworkLookup{}, err
		}

		install, err := r.service.GetInstall(name)
		if err != nil {
			return NetworkLookup{}, err
		}
		return NetworkLookup{Install: install}, nil
	})
}

// UpdateNetwork persists local network configuration and converges the
// network under it: a running network whose stored configuration changed
// is restarted. It returns the updated record. The record is already
// stored when convergence runs, so a converge failure is logged and
// retried on the next pass rather than failing the operation.
func (r *Runtime) UpdateNetwork(
	ctx context.Context,
	name string,
	opts service.NetworkOptions,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (*service.Network, error) {
		if err := r.service.UpdateNetwork(name, opts); err != nil {
			return nil, err
		}

		if err := r.convergeName(name); err != nil {
			r.log.Error(
				"converge network failed",
				"network", name,
				"err", err,
			)
		}

		return r.service.GetNetwork(name)
	})
}

// SetNetworkEnabled records whether the network should be running and
// converges it, returning the resulting status. The intent is persisted
// unconditionally, so a network that cannot start stays enabled and its
// status explains why, rather than the operation failing and reverting
// the operator's intent.
func (r *Runtime) SetNetworkEnabled(
	ctx context.Context,
	name string,
	enabled bool,
) (
	NetworkStatus,
	error,
) {
	return executeValue(r, ctx, func() (NetworkStatus, error) {
		if err := r.service.SetNetworkEnabled(name, enabled); err != nil {
			return NetworkStatus{}, err
		}

		if err := r.convergeName(name); err != nil {
			r.log.Error(
				"converge network failed",
				"network", name,
				"err", err,
			)
		}

		return r.getNetworkStatus(name)
	})
}

// UninstallNetwork removes the network's record and converges: the
// record is gone, so a network still running is taken down with it.
func (r *Runtime) UninstallNetwork(
	ctx context.Context,
	name string,
) error {
	return execute(r, ctx, func() error {
		if err := r.service.UninstallNetwork(name); err != nil {
			return err
		}

		if err := r.convergeName(name); err != nil {
			r.log.Error(
				"converge network failed",
				"network", name,
				"err", err,
			)
		}

		return nil
	})
}

// Sync fetches the network snapshot from the server on demand, outside
// the network's own sync cadence, and returns the updated record. It
// returns ErrNotFound when neither a network nor an install exists, and
// ErrNetworkNotEnabled when it exists but is not running, since there is
// no tunnel to fetch over.
func (r *Runtime) Sync(
	ctx context.Context,
	name string,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (*service.Network, error) {
		_, err := r.service.GetNetwork(name)
		if errors.Is(err, service.ErrNotFound) {
			_, err = r.service.GetInstall(name)
			if err == nil {
				return nil, service.ErrNetworkNotEnabled
			}
		}
		if err != nil {
			return nil, err
		}

		network, isRunning := r.running[name]

		if !isRunning {
			return nil, service.ErrNetworkNotEnabled
		}

		// Stop outbound requests on caller or runtime cancellation.
		requestCtx, cancel := context.WithCancel(ctx)
		removeShutdownHook := context.AfterFunc(r.ctx, cancel)
		defer removeShutdownHook()
		defer cancel()

		if err := network.sync(requestCtx); err != nil {
			return nil, err
		}

		return r.service.GetNetwork(name)
	})
}

// Converge applies a network's saved intent through the runtime owner.
func (r *Runtime) Converge(
	ctx context.Context,
	name string,
) error {
	return execute(r, ctx, func() error {
		return r.convergeName(name)
	})
}

// GetNetworkTopology returns the last synchronized topology joined with
// live peer connectivity. It remains available while the network is
// disabled or offline.
func (r *Runtime) GetNetworkTopology(
	ctx context.Context,
	name string,
) (
	NetworkTopology,
	error,
) {
	return executeValue(r, ctx, func() (NetworkTopology, error) {
		cached, err := r.service.GetNetworkTopology(name)
		if err != nil {
			return NetworkTopology{}, err
		}

		record, err := r.service.GetNetwork(name)
		if err != nil {
			return NetworkTopology{}, err
		}

		peers, err := r.service.ListPeers(name)
		if err != nil {
			return NetworkTopology{}, err
		}

		livePeerStatuses, err := r.getLivePeerStatuses(name)
		if err != nil {
			return NetworkTopology{}, err
		}

		now := r.clock()
		statuses := joinPeerStatuses(peers, livePeerStatuses, now)

		_, running := r.running[name]

		connected := make(map[string]bool, len(statuses)+2)
		for _, status := range statuses {
			connected[status.Name] = status.Connected
		}
		serverConnected := false
		if devicePeerStatus, ok := livePeerStatuses[record.Server.PublicKey]; ok {
			serverConnected = peerHealthy(devicePeerStatus, now)
		}
		connected[serverPeerName] = serverConnected

		if cached.View.SubjectPeer != "" {
			if _, ok := connected[cached.View.SubjectPeer]; !ok {
				connected[cached.View.SubjectPeer] = running
			}
		}

		return NetworkTopology{
			View:        cached.View,
			GeneratedAt: cached.GeneratedAt,
			SyncedAt:    cached.SyncedAt,
			Connected:   connected,
		}, nil
	})
}

// convergeName brings one network in line with its persisted intent: an
// enabled network that is not running is started, a running network that
// is no longer enabled — or no longer installed — is stopped, and a
// running network whose stored configuration has changed is restarted. A
// start failure is recorded as the network's reason and returned; the
// persisted intent is never touched.
func (r *Runtime) convergeName(
	name string,
) error {
	record, err := r.service.GetNetwork(name)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound):
			return r.removeNetwork(name)
		default:
			return err
		}
	}
	return r.converge(record)
}

// converge applies one network record under the runtime owner.
func (r *Runtime) converge(
	record *service.Network,
) error {
	if r.ctx.Err() != nil {
		return nil
	}

	network, isRunning := r.running[record.Name]

	switch {
	case !record.Enabled:
		// Stopped by intent; a network still running comes down with it.
		delete(r.reasons, record.Name)
		if isRunning {
			return r.stopNetwork(record.Name, network)
		}

	case isRunning && network.closing:
		// Finish a previous cleanup attempt before starting again, even if
		// the desired configuration has returned to its original value.
		return r.restartNetwork(record, network)

	case isRunning && network.isDiverged(record):
		// The stored configuration no longer describes the running
		// tunnel, so the network restarts under the new one.
		return r.restartNetwork(record, network)

	case isRunning:
		// Already where it should be; reapply the cached peer set.
		if err := network.reconcile(); err != nil {
			r.log.Warn(
				"reconcile failed",
				"network", record.Name,
				"err", err,
			)
		}
		break // everything is good

	default:
		return r.startNetwork(record)
	}

	return nil
}

// startNetwork brings a network up under its record and into the
// running set, or records the failure as its reason to be retried on
// the next pass.
func (r *Runtime) startNetwork(
	record *service.Network,
) error {
	if err := r.releaseEnrollmentsUsing(record.InterfaceName); err != nil {
		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	network, err := newNetwork(networkOptions{
		record:         record,
		service:        r.service,
		wireguard:      r.wireguard,
		dns:            r.dns,
		httpClient:     r.httpClient,
		clock:          r.clock,
		log:            r.log.With("network", record.Name),
		syncInterval:   r.syncInterval,
		scanInterval:   r.scanInterval,
		reportInterval: r.reportInterval,
	})
	if err != nil {
		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	// Own the instance before startup; cleanup decides when to forget it.
	r.running[record.Name] = network

	if err := network.start(r.ctx); err != nil {
		cleanupErr := r.stopNetwork(record.Name, network)
		err = errors.Join(err, cleanupErr)

		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	delete(r.reasons, record.Name)

	r.log.Info(
		"network started",
		"network", record.Name,
		"interface", record.InterfaceName,
	)

	return nil
}

// restartNetwork stops a diverged network and brings it back up under
// the new record.
func (r *Runtime) restartNetwork(
	record *service.Network,
	network *Network,
) error {
	if err := r.stopNetwork(record.Name, network); err != nil {
		return err
	}

	return r.startNetwork(record)
}

// stopNetwork forgets a network only after cleanup succeeds.
func (r *Runtime) stopNetwork(
	name string,
	network *Network,
) error {
	network.closing = true
	if err := network.stop(); err != nil {
		err = fmt.Errorf("stop network %q: %w", name, err)
		r.reasons[name] = err.Error()
		return err
	}

	delete(r.running, name)
	delete(r.reasons, name)

	r.log.Info(
		"network stopped",
		"network", name,
	)

	return nil
}

// removeNetwork stops a network that is no longer installed and forgets it.
func (r *Runtime) removeNetwork(
	name string,
) error {

	network, isRunning := r.running[name]
	if !isRunning {
		delete(r.reasons, name)
		return nil
	}

	return r.stopNetwork(name, network)
}

// closeNetworks retains failed cleanup and reports it to the Run caller.
func (r *Runtime) closeNetworks() error {
	var errs []error
	for name, network := range r.running {
		if err := r.stopNetwork(name, network); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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

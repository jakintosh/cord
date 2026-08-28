// Package runtime supervises the client networks a cord daemon is
// running. It is the only place a network is brought up: it reads the
// desired state the service persists, compares it against the networks
// actually running in this process, and converges the difference.
// Failures never change the desired state: startup failures are recorded
// as a reason, and failures of running work are recorded as health.
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
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultInterval is how often the runtime runs a full converge pass
// when nothing wakes it earlier. It is also the retry interval for a
// network that failed to start.
const DefaultInterval = 30 * time.Second

const (
	HealthInactive = "inactive"
	HealthHealthy  = "healthy"
	HealthDegraded = "degraded"
)

// Options configures the runtime.
type Options struct {
	// Service is the domain core holding the desired state.
	Service *service.Service

	// WireGuard is the manager used to create network devices.
	WireGuard *wireguard.Manager

	// HTTPClient backs every call to a server API. Nil uses the
	// default transport.
	HTTPClient *http.Client

	// Wake carries the names of networks whose state changed, so the
	// runtime can converge them without waiting for the next tick.
	Wake <-chan string

	// Interval is the period between full converge passes. Defaults to
	// DefaultInterval when zero.
	Interval time.Duration

	// SyncInterval, ScanInterval, and ReportInterval are the cadences of
	// a running network's activities. Each defaults to its package
	// constant when zero.
	SyncInterval   time.Duration
	ScanInterval   time.Duration
	ReportInterval time.Duration

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives runtime diagnostics. Nil discards everything.
	Logger *slog.Logger
}

// Runtime reconciles the running networks toward the persisted desired
// state. It is safe for concurrent use.
type Runtime struct {
	service    *service.Service
	wireguard  *wireguard.Manager
	httpClient *http.Client
	wake       <-chan string
	interval   time.Duration
	clock      func() time.Time
	log        *slog.Logger

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	mu      sync.Mutex
	running map[string]*Network
	reasons map[string]string
	ctx     context.Context
	cancel  context.CancelFunc

	loop sync.WaitGroup
}

// Status is a snapshot of every network known to the client daemon.
type Status struct {
	Health   string
	Networks []NetworkStatus
}

// NetworkStatus is the operator-facing state of an installed network:
// persisted intent, actual process state, and the health of running work.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Running   bool
	Reason    string
	Health    string
	Reconcile ActivityStatus
	Sync      ActivityStatus
	Scan      ActivityStatus
	Report    ActivityStatus
}

// ActivityStatus describes the latest result of one runtime activity.
// Interval is zero for work without a periodic schedule.
type ActivityStatus struct {
	Interval      time.Duration
	LastAttemptAt time.Time
	LastSuccessAt time.Time
	Error         string
}

func (s *ActivityStatus) record(
	at time.Time,
	err error,
) {
	s.LastAttemptAt = at
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.LastSuccessAt = at
	s.Error = ""
}

// PeerStatus is a cached peer joined with its live WireGuard device
// state. The device fields are zero-valued when the network isn't
// currently running.
type PeerStatus struct {
	Name          string
	Route         string
	Endpoint      string
	LastHandshake time.Time
	Connected     bool
}

// NetworkTopology is the network's cached topology joined with live peer
// connectivity for display. Connected maps a peer name to its live
// state; peers absent from the map have no known connectivity. The
// subject peer is always present: its connectivity is whether the
// network is running.
type NetworkTopology struct {
	View        topology.View
	GeneratedAt time.Time
	SyncedAt    time.Time
	Connected   map[string]bool
}

// New returns a runtime that is not yet running. Call Start to converge
// the persisted state and begin the periodic pass.
func New(
	opts Options,
) (
	*Runtime,
	error,
) {
	if opts.Service == nil {
		return nil, fmt.Errorf("client: service required")
	}
	if opts.WireGuard == nil {
		return nil, fmt.Errorf("client: wireguard manager required")
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}

	syncInterval := intervalOr(opts.SyncInterval, SyncInterval)
	scanInterval := intervalOr(opts.ScanInterval, ScanInterval)
	reportInterval := intervalOr(opts.ReportInterval, ReportInterval)

	return &Runtime{
		service:        opts.Service,
		wireguard:      opts.WireGuard,
		httpClient:     opts.HTTPClient,
		wake:           opts.Wake,
		interval:       interval,
		clock:          clock,
		log:            log,
		syncInterval:   syncInterval,
		scanInterval:   scanInterval,
		reportInterval: reportInterval,
		running:        make(map[string]*Network),
		reasons:        make(map[string]string),
	}, nil
}

// Start converges every installed network once, then keeps converging:
// on a wake from the service, and on every tick of the interval. It
// returns an error only when the desired state cannot be read at all;
// a network that fails to start is recorded and retried.
func (r *Runtime) Start(
	ctx context.Context,
) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return fmt.Errorf("client: runtime already started")
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.mu.Unlock()

	if err := r.ConvergeAll(); err != nil {
		r.cancel()
		return err
	}

	r.loop.Add(1)
	go r.run()

	return nil
}

// Stop ends the periodic pass, waits for work in flight, and stops every
// running network. The runtime cannot be started again.
func (r *Runtime) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.loop.Wait()

	r.mu.Lock()
	networks := r.running
	r.running = make(map[string]*Network)
	r.mu.Unlock()

	for name, network := range networks {
		if err := network.stop(); err != nil {
			r.log.Warn("stop network failed", "network", name, "err", err)
		}
	}
}

// Converge brings one network in line with its persisted intent: an
// enabled network that is not running is started, a running network that
// is no longer enabled — or no longer installed — is stopped, and a
// running network whose stored configuration has changed is restarted. A
// start failure is recorded as the network's reason and returned; the
// persisted intent is never touched.
func (r *Runtime) Converge(
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

// ConvergeAll converges every installed network and stops anything still
// running that no longer has a record. Per-network failures are recorded
// and logged; only a failure to read the desired state is returned.
func (r *Runtime) ConvergeAll() error {
	records, err := r.service.ListNetworks()
	if err != nil {
		return err
	}

	installed := make(map[string]bool, len(records))
	for _, record := range records {
		installed[record.Name] = true
		if err := r.converge(record); err != nil {
			r.log.Error(
				"converge failed",
				"network",
				record.Name,
				"err",
				err,
			)
		}
	}

	r.mu.Lock()
	var uninstalled []string
	for name := range r.running {
		if !installed[name] {
			uninstalled = append(uninstalled, name)
		}
	}
	r.mu.Unlock()

	for _, name := range uninstalled {
		if err := r.removeNetwork(name); err != nil {
			r.log.Warn(
				"stop uninstalled network failed",
				"network",
				name,
				"err",
				err,
			)
		}
	}
	return nil
}

// UpdateNetwork persists local network configuration and converges the
// network under it: a running network whose stored configuration changed
// is restarted. It returns the updated record. The record is already
// stored when convergence runs, so a converge failure is logged and
// retried on the next pass rather than failing the operation.
func (r *Runtime) UpdateNetwork(
	name string,
	opts service.NetworkOptions,
) (
	*service.Network,
	error,
) {
	if err := r.service.UpdateNetwork(name, opts); err != nil {
		return nil, err
	}

	if err := r.Converge(name); err != nil {
		r.log.Error(
			"converge network failed",
			"network",
			name,
			"err",
			err,
		)
	}

	return r.service.GetNetwork(name)
}

// SetNetworkEnabled records whether the network should be running and
// converges it, returning the resulting status. The intent is persisted
// unconditionally, so a network that cannot start stays enabled and its
// status explains why, rather than the operation failing and reverting
// the operator's intent.
func (r *Runtime) SetNetworkEnabled(
	name string,
	enabled bool,
) (
	NetworkStatus,
	error,
) {
	if err := r.service.SetNetworkEnabled(name, enabled); err != nil {
		return NetworkStatus{}, err
	}

	if err := r.Converge(name); err != nil {
		r.log.Error(
			"converge network failed",
			"network",
			name,
			"err",
			err,
		)
	}

	return r.getNetworkStatus(name)
}

// UninstallNetwork removes the network's record and converges: the
// record is gone, so a network still running is taken down with it.
func (r *Runtime) UninstallNetwork(
	name string,
) error {
	if err := r.service.UninstallNetwork(name); err != nil {
		return err
	}

	if err := r.Converge(name); err != nil {
		r.log.Error(
			"converge network failed",
			"network",
			name,
			"err",
			err,
		)
	}

	return nil
}

// Sync fetches the network snapshot from the server on demand, outside
// the network's own sync cadence, and returns the updated record. It
// returns ErrNetworkNotEnabled when the network is not running, since
// there is no tunnel to fetch over.
func (r *Runtime) Sync(
	name string,
) (
	*service.Network,
	error,
) {
	r.mu.Lock()
	network, isRunning := r.running[name]
	r.mu.Unlock()

	if !isRunning {
		return nil, service.ErrNetworkNotEnabled
	}
	if err := network.sync(); err != nil {
		return nil, err
	}
	return r.service.GetNetwork(name)
}

// GetStatus joins the persisted intent with what this process is actually
// doing. It is the single source of operator-facing network state.
func (r *Runtime) GetStatus() (
	Status,
	error,
) {
	records, err := r.service.ListNetworks()
	if err != nil {
		return Status{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	statuses := make([]NetworkStatus, len(records))
	for i, record := range records {
		statuses[i] = r.parseStatusOf(record)
	}

	return Status{
		Health:   statusHealth(statuses),
		Networks: statuses,
	}, nil
}

// GetPeerStatus returns the network's cached peers joined with live device
// state. A network that is installed but not running reports its cached
// peers with zero-valued device fields rather than an error.
func (r *Runtime) GetPeerStatus(
	network string,
) (
	[]PeerStatus,
	error,
) {
	peers, err := r.service.ListPeers(network)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	running, isRunning := r.running[network]
	r.mu.Unlock()

	livePeerStatuses := make(map[string]wireguard.PeerStatus)
	if isRunning {
		peerStatuses, err := running.getPeerStatuses()
		if err != nil {
			return nil, err
		}
		for _, peer := range peerStatuses {
			livePeerStatuses[peer.PublicKey.String()] = peer
		}
	}

	now := r.clock()
	statuses := make([]PeerStatus, len(peers))
	for i, peer := range peers {
		status := PeerStatus{
			Name:  peer.Name,
			Route: peer.Route,
		}
		if devicePeerStatus, ok := livePeerStatuses[peer.PublicKey]; ok {
			if devicePeerStatus.Endpoint != nil {
				status.Endpoint = devicePeerStatus.Endpoint.String()
			}
			status.LastHandshake = devicePeerStatus.LastHandshake
			status.Connected = peerHealthy(devicePeerStatus, now)
		}
		statuses[i] = status
	}
	return statuses, nil
}

// GetNetworkTopology returns the last synchronized topology joined with
// live peer connectivity. It remains available while the network is
// disabled or offline.
func (r *Runtime) GetNetworkTopology(
	name string,
) (
	NetworkTopology,
	error,
) {
	cached, err := r.service.GetNetworkTopology(name)
	if err != nil {
		return NetworkTopology{}, err
	}

	statuses, err := r.GetPeerStatus(name)
	if err != nil {
		return NetworkTopology{}, err
	}

	r.mu.Lock()
	_, running := r.running[name]
	r.mu.Unlock()

	connected := make(map[string]bool, len(statuses)+1)
	for _, status := range statuses {
		connected[status.Name] = status.Connected
	}
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
}

// run is the periodic pass: it converges on every wake and every tick
// until the run context is cancelled.
func (r *Runtime) run() {
	defer r.loop.Done()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {

		case <-r.ctx.Done():
			return

		case name := <-r.wake:
			if err := r.Converge(name); err != nil {
				r.log.Error(
					"converge failed",
					"network",
					name,
					"err",
					err,
				)
			}

		case <-ticker.C:
			if err := r.ConvergeAll(); err != nil {
				r.log.Error(
					"converge pass failed",
					"err",
					err,
				)
			}
		}
	}
}

// converge applies one network record to the running set. It holds the
// runtime lock for the whole transition, so concurrent callers see a
// consistent running set and never race to start the same network.
func (r *Runtime) converge(
	record *service.Network,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ctx != nil && r.ctx.Err() != nil {
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

	case isRunning && network.isDiverged(record):
		// The stored configuration no longer describes the running
		// tunnel, so the network restarts under the new one.
		return r.restartNetwork(record, network)

	case isRunning:
		// Already where it should be; reapply the cached peer set.
		if err := network.reconcile(); err != nil {
			r.log.Warn(
				"reconcile failed",
				"network",
				record.Name,
				"err",
				err,
			)
		}

	default:
		return r.startNetwork(record)
	}

	return nil
}

// startNetwork brings a network up under its record and into the
// running set, or records the failure as its reason to be retried on
// the next pass. Callers hold r.mu.
func (r *Runtime) startNetwork(
	record *service.Network,
) error {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	network := &Network{
		record:         record,
		service:        r.service,
		clock:          r.clock,
		log:            r.log.With("network", record.Name),
		syncInterval:   r.syncInterval,
		scanInterval:   r.scanInterval,
		reportInterval: r.reportInterval,
		syncStatus:     ActivityStatus{Interval: r.syncInterval},
		scanStatus:     ActivityStatus{Interval: r.scanInterval},
		reportStatus:   ActivityStatus{Interval: r.reportInterval},
	}
	if err := network.start(
		ctx,
		r.wireguard,
		r.httpClient,
	); err != nil {
		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	r.running[record.Name] = network
	delete(r.reasons, record.Name)

	r.log.Info(
		"network started",
		"network",
		record.Name,
		"interface",
		record.InterfaceName,
	)

	return nil
}

// restartNetwork stops a diverged network and brings it back up under
// the new record. Callers hold r.mu.
func (r *Runtime) restartNetwork(
	record *service.Network,
	network *Network,
) error {
	delete(r.running, record.Name)
	if err := network.stop(); err != nil {
		r.log.Warn(
			"restart: stop network failed",
			"network",
			record.Name,
			"err",
			err,
		)
	}

	return r.startNetwork(record)
}

// stopNetwork takes a running network down and forgets it. Callers hold
// r.mu.
func (r *Runtime) stopNetwork(
	name string,
	network *Network,
) error {
	delete(r.running, name)
	delete(r.reasons, name)
	if err := network.stop(); err != nil {
		return fmt.Errorf("stop network %q: %w", name, err)
	}

	r.log.Info(
		"network stopped",
		"network",
		name,
	)

	return nil
}

// removeNetwork stops a network that is no longer installed and forgets it.
func (r *Runtime) removeNetwork(
	name string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	network, isRunning := r.running[name]
	if !isRunning {
		delete(r.reasons, name)
		return nil
	}

	return r.stopNetwork(name, network)
}

// getNetworkStatus reports the status of one network by name.
func (r *Runtime) getNetworkStatus(
	name string,
) (
	NetworkStatus,
	error,
) {
	record, err := r.service.GetNetwork(name)
	if err != nil {
		return NetworkStatus{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.parseStatusOf(record), nil
}

// parseStatusOf joins one persisted record with the running set. Callers hold
// r.mu.
func (r *Runtime) parseStatusOf(
	record *service.Network,
) NetworkStatus {
	network, isRunning := r.running[record.Name]
	status := NetworkStatus{
		Name:    record.Name,
		Enabled: record.Enabled,
		Running: isRunning,
		Reason:  r.reasons[record.Name],
		Sync:    ActivityStatus{Interval: r.syncInterval},
		Scan:    ActivityStatus{Interval: r.scanInterval},
		Report:  ActivityStatus{Interval: r.reportInterval},
	}
	if isRunning {
		status.Reconcile, status.Sync, status.Scan, status.Report = network.getActivityStatuses()
	}
	status.Health = networkHealth(status)
	return status
}

func networkHealth(
	status NetworkStatus,
) string {
	if !status.Enabled && !status.Running {
		return HealthInactive
	}
	if status.Enabled != status.Running {
		return HealthDegraded
	}
	activities := []ActivityStatus{
		status.Reconcile,
		status.Sync,
		status.Scan,
		status.Report,
	}
	for _, activity := range activities {
		if activity.Error != "" {
			return HealthDegraded
		}
	}
	return HealthHealthy
}

func statusHealth(
	statuses []NetworkStatus,
) string {
	for _, status := range statuses {
		if status.Health == HealthDegraded {
			return HealthDegraded
		}
	}
	return HealthHealthy
}

// intervalOr returns value, or fallback when value is not positive.
func intervalOr(
	value time.Duration,
	fallback time.Duration,
) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

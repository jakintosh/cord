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
	"sync/atomic"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultInterval is how often the runtime repairs resources and retries
// networks that failed to start.
const DefaultInterval = 30 * time.Second

var (
	// ErrAlreadyRun rejects concurrent runs and reuse of a finished runtime.
	ErrAlreadyRun = errors.New("runtime has already run")

	// ErrNotRunning is returned after Run stops accepting operations.
	ErrNotRunning = errors.New("runtime is not running")
)

const (
	HealthInactive = "inactive"
	HealthHealthy  = "healthy"
	HealthDegraded = "degraded"

	serverPeerName = "cord-server"
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

// Runtime executes commands and maintenance in one owner goroutine. Public
// methods are safe for concurrent use; calls before Run wait for it to start.
type Runtime struct {
	service    *service.Service
	wireguard  *wireguard.Manager
	httpClient *http.Client
	interval   time.Duration
	clock      func() time.Time
	log        *slog.Logger

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	operations chan operation
	done       chan struct{}
	started    atomic.Bool

	// Only Run accesses the state below and the networks it owns.
	running     map[string]*Network
	enrollments map[string]*Enrollment
	reasons     map[string]string
	ctx         context.Context

	reconcileStatus ActivityStatus
	cleanupStatus   ActivityStatus
}

// Status collects network state and pending installations in one runtime command.
type Status struct {
	Snapshot ResourceSnapshot
	Health   string
	Networks []NetworkStatus
	Installs []*service.Install
}

// NetworkLookup contains exactly one record on a successful lookup.
type NetworkLookup struct {
	Network *service.Network
	Install *service.Install
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

// NetworkStatus is the operator-facing state of an installed network:
// persisted intent, actual process state, and the health of running work.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Reason    string
	Health    string
	Reconcile ActivityStatus
	Sync      ActivityStatus
	Scan      ActivityStatus
	Report    ActivityStatus
}

// ActivityStatus holds the schedule and latest result of one runtime activity.
// Interval is zero for work without a periodic schedule.
type ActivityStatus struct {
	NextAttemptAt time.Time
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

// New returns a runtime that is not yet running. Call Run to converge
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
		interval:       interval,
		clock:          clock,
		log:            log,
		syncInterval:   syncInterval,
		scanInterval:   scanInterval,
		reportInterval: reportInterval,
		running:        make(map[string]*Network),
		enrollments:    make(map[string]*Enrollment),
		reasons:        make(map[string]string),
		operations:     make(chan operation),
		done:           make(chan struct{}),
	}, nil
}

// Run executes operations and maintenance until cancellation, then closes owned
// networks before returning. A runtime is single-use. The supplied context owns
// the resource lifetime; individual commands never become resource parents.
func (r *Runtime) Run(
	ctx context.Context,
) (
	err error,
) {
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyRun
	}

	r.ctx = ctx
	defer func() {
		close(r.done)
		err = errors.Join(err, r.closeEnrollments(), r.closeNetworks())
	}()

	if ctx.Err() != nil {
		return nil
	}

	if err := r.convergeAll(); err != nil {
		return err
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	activityTimer := time.NewTimer(0)
	defer activityTimer.Stop()

	for {
		activityTimer.Stop()
		var activityWake <-chan time.Time
		if next := r.nextActivity(); !next.IsZero() {
			activityTimer.Reset(next.Sub(r.clock()))
			activityWake = activityTimer.C
		}

		select {
		case <-ctx.Done():
			return nil

		case <-activityWake:
			for _, network := range r.running {
				if ctx.Err() != nil {
					return nil
				}
				network.runDueActivities()
			}

		case op := <-r.operations:
			if ctx.Err() != nil {
				return nil
			}

			if op.ctx.Err() != nil {
				continue
			}

			op.run()

		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}

			if err := r.convergeAll(); err != nil {
				r.log.Error(
					"converge pass failed",
					"err", err,
				)
			}
		}
	}
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
// returns ErrNetworkNotEnabled when the network is not running, since
// there is no tunnel to fetch over.
func (r *Runtime) Sync(
	ctx context.Context,
	name string,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (*service.Network, error) {
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

// ConvergeAll requests a full maintenance pass through the runtime owner.
func (r *Runtime) ConvergeAll(
	ctx context.Context,
) error {
	return execute(r, ctx, func() error {
		return r.convergeAll()
	})
}

// GetStatus joins the persisted intent with what this process is actually
// doing. Storage read failures are reported on the resource root so that
// owned resources remain observable when their saved summaries are unavailable.
func (r *Runtime) GetStatus(
	ctx context.Context,
) (
	Status,
	error,
) {
	return executeValue(r, ctx, func() (Status, error) {
		records, networkErr := r.service.ListNetworks()

		statuses := make([]NetworkStatus, len(records))
		for i, record := range records {
			statuses[i] = r.parseStatusOf(record)
		}

		installs, installErr := r.service.ListInstalls()

		tree := r.status()
		if err := errors.Join(networkErr, installErr); err != nil {
			tree.Error = fmt.Sprintf("read saved state: %v", err)
			tree.updateHealth()
		}

		health := statusHealth(statuses)
		if health == HealthDegraded || tree.Health == HealthDegraded {
			health = HealthDegraded
			tree.Health = HealthDegraded
		}

		return Status{
			Snapshot: tree,
			Health:   health,
			Networks: statuses,
			Installs: installs,
		}, nil
	})
}

// GetPeerStatus returns the network's cached peers joined with live device
// state. A network that is installed but not running reports its cached
// peers with zero-valued device fields rather than an error.
func (r *Runtime) GetPeerStatus(
	ctx context.Context,
	network string,
) (
	[]PeerStatus,
	error,
) {
	return executeValue(r, ctx, func() ([]PeerStatus, error) {
		peers, err := r.service.ListPeers(network)
		if err != nil {
			return nil, err
		}

		livePeerStatuses, err := r.getLivePeerStatuses(network)
		if err != nil {
			return nil, err
		}

		return joinPeerStatuses(peers, livePeerStatuses, r.clock()), nil
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

// nextActivity finds the earliest work across active networks.
func (r *Runtime) nextActivity() time.Time {
	var next time.Time
	for _, network := range r.running {
		due := network.nextActivity()
		if due.IsZero() {
			continue
		}
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return next
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

// convergeAll converges every installed network and stops anything still
// running that no longer has a record. Per-network failures are recorded
// and logged; only a failure to read the desired state is returned.
func (r *Runtime) convergeAll() (
	err error,
) {
	defer func() {
		r.reconcileStatus.Interval = r.interval
		r.reconcileStatus.record(r.clock(), err)
	}()

	if err := r.closeEnrollments(); err != nil {
		r.log.Warn(
			"enrollment cleanup failed",
			"err", err,
		)
	}

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
				"network", record.Name,
				"err", err,
			)
		}
	}

	var uninstalled []string
	for name := range r.running {
		if !installed[name] {
			uninstalled = append(uninstalled, name)
		}
	}

	for _, name := range uninstalled {
		if err := r.removeNetwork(name); err != nil {
			r.log.Warn(
				"stop uninstalled network failed",
				"network", name,
				"err", err,
			)
		}
	}
	return nil
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

	return r.parseStatusOf(record), nil
}

// parseStatusOf joins one persisted record with the owned network state.
func (r *Runtime) parseStatusOf(
	record *service.Network,
) NetworkStatus {
	network, isRunning := r.running[record.Name]
	status := NetworkStatus{
		Name:    record.Name,
		Enabled: record.Enabled,
		Reason:  r.reasons[record.Name],
		Sync:    ActivityStatus{Interval: r.syncInterval},
		Scan:    ActivityStatus{Interval: r.scanInterval},
		Report:  ActivityStatus{Interval: r.reportInterval},
	}

	if isRunning {
		status.Reconcile, status.Sync, status.Scan, status.Report = network.getActivityStatuses()
	}

	status.Health = networkHealth(status, isRunning)
	if isRunning && network.status().Health == HealthDegraded {
		status.Health = HealthDegraded
	}

	return status
}

// getLivePeerStatuses returns the running network's live WireGuard peers
// indexed by public key. An installed network that is not running has no
// live device state and returns an empty map without an error.
func (r *Runtime) getLivePeerStatuses(
	network string,
) (
	map[string]wireguard.PeerStatus,
	error,
) {
	running, isRunning := r.running[network]

	if !isRunning {
		return nil, nil
	}

	peerStatuses, err := running.getPeerStatuses()
	if err != nil {
		return nil, err
	}
	livePeerStatuses := make(map[string]wireguard.PeerStatus, len(peerStatuses))
	for _, peer := range peerStatuses {
		livePeerStatuses[peer.PublicKey.String()] = peer
	}
	return livePeerStatuses, nil
}

// operation carries work to the sole owner of runtime state.
type operation struct {
	ctx context.Context
	run func()
}

// executeValue submits work and waits for its result. Owner-loop code calls
// private methods directly; submitting another command would deadlock.
// Cancellation releases the caller without undoing work already performed.
func executeValue[T any](
	r *Runtime,
	ctx context.Context,
	fn func() (T, error),
) (
	T,
	error,
) {
	type result struct {
		value T
		err   error
	}
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	// A buffered reply lets the owner finish after the caller stops waiting.
	reply := make(chan result, 1)
	op := operation{
		ctx: ctx,
		run: func() {
			value, err := fn()
			reply <- result{
				value: value,
				err:   err,
			}
		},
	}

	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-r.done:
		return zero, ErrNotRunning
	case r.operations <- op:
	}

	select {
	case result := <-reply:
		return result.value, result.err
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-r.done:
		return zero, ErrNotRunning
	}
}

func execute(
	r *Runtime,
	ctx context.Context,
	fn func() error,
) error {
	_, err := executeValue(r, ctx, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// joinPeerStatuses combines cached peer records with one live device
// snapshot.
func joinPeerStatuses(
	peers []*service.Peer,
	livePeerStatuses map[string]wireguard.PeerStatus,
	now time.Time,
) []PeerStatus {
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
	return statuses
}

func networkHealth(
	status NetworkStatus,
	owned bool,
) string {
	if status.Reason != "" {
		return HealthDegraded
	}

	if !status.Enabled && !owned {
		return HealthInactive
	}

	if status.Enabled != owned {
		return HealthDegraded
	}

	for _, activity := range []ActivityStatus{
		status.Reconcile,
		status.Sync,
		status.Scan,
		status.Report,
	} {
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

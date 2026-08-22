// Package runtime supervises the server networks a cord daemon is
// running. It is the only place a network is brought up: it reads the
// desired state the service persists, compares it against the networks
// actually running in this process, and converges the difference.
// Failures never change the desired state: startup failures are recorded
// as a reason, and failures of running work are recorded as health.
package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	inviteapi "git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	peerapi "git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
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

	// Peer serves each running network's main plane.
	Peer *peerapi.API

	// Invite serves each running network's invite plane.
	Invite *inviteapi.API

	// Wake carries the names of networks whose state changed, so the
	// runtime can converge them without waiting for the next tick.
	Wake <-chan string

	// Interval is the period between full converge passes. Defaults to
	// DefaultInterval when zero.
	Interval time.Duration

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives runtime diagnostics. Nil discards everything.
	Logger *slog.Logger
}

// Runtime reconciles the running networks toward the persisted desired
// state. It is safe for concurrent use.
type Runtime struct {
	service   *service.Service
	wireguard *wireguard.Manager
	peer      *peerapi.API
	invite    *inviteapi.API
	wake      <-chan string
	interval  time.Duration
	clock     func() time.Time
	log       *slog.Logger

	mu      sync.Mutex
	running map[string]*Network
	reasons map[string]string
	ctx     context.Context
	cancel  context.CancelFunc

	loop sync.WaitGroup
}

// Status is a snapshot of every network known to the server daemon.
type Status struct {
	Health   string
	Networks []NetworkStatus
}

// NetworkStatus is the operator-facing state of a server network:
// persisted intent, actual process state, and the health of running work.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Running   bool
	Reason    string
	Health    string
	Reconcile ActivityStatus
	MainAPI   ActivityStatus
	InviteAPI ActivityStatus
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

// New returns a runtime that is not yet running. Call Start to converge
// the persisted state and begin the periodic pass.
func New(
	opts Options,
) (
	*Runtime,
	error,
) {
	if opts.Service == nil {
		return nil, fmt.Errorf("server: service required")
	}

	if opts.WireGuard == nil {
		return nil, fmt.Errorf("server: wireguard manager required")
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

	return &Runtime{
		service:   opts.Service,
		wireguard: opts.WireGuard,
		peer:      opts.Peer,
		invite:    opts.Invite,
		wake:      opts.Wake,
		interval:  interval,
		clock:     clock,
		log:       log,
		running:   make(map[string]*Network),
		reasons:   make(map[string]string),
	}, nil
}

// Start converges every persisted network once, then keeps converging:
// on a wake from the service, and on every tick of the interval. It
// returns an error only when the desired state cannot be read at all;
// a network that fails to start is recorded and retried.
func (r *Runtime) Start(
	ctx context.Context,
) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return fmt.Errorf("server: runtime already started")
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
			r.log.Warn(
				"stop network failed",
				"network",
				name,
				"err",
				err,
			)
		}
	}
}

// Converge brings one network in line with its persisted intent: an
// enabled network that is not running is started, a running network that
// is no longer enabled is stopped, and a network already where it should
// be has its peer sets reconciled. A start failure is recorded as the
// network's reason and returned; the persisted intent is never touched.
func (r *Runtime) Converge(
	name string,
) error {
	network, err := r.service.GetNetwork(name)
	if err != nil {
		return err
	}
	return r.converge(network)
}

// ConvergeAll converges every persisted network. Per-network failures
// are recorded and logged; only a failure to read the desired state is
// returned.
func (r *Runtime) ConvergeAll() error {
	networks, err := r.service.ListNetworks()
	if err != nil {
		return err
	}

	for _, network := range networks {
		if err := r.converge(network); err != nil {
			r.log.Error(
				"converge failed",
				"network",
				network.Name,
				"err",
				err,
			)
		}
	}

	return nil
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

// Status joins the persisted intent with what this process is actually
// doing. It is the single source of operator-facing state.
func (r *Runtime) Status() (
	Status,
	error,
) {
	networks, err := r.service.ListNetworks()
	if err != nil {
		return Status{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	statuses := make([]NetworkStatus, len(networks))
	for i, network := range networks {
		statuses[i] = r.parseStatusOf(network)
	}

	return Status{
		Health:   statusHealth(statuses),
		Networks: statuses,
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
	case !record.Enabled && isRunning:
		// Disabled but still up; bring it down.
		delete(r.reasons, record.Name)
		return r.stopNetwork(record.Name, network)

	case !record.Enabled && !isRunning:
		// Disabled and already down; forget any stale reason.
		delete(r.reasons, record.Name)

	case record.Enabled && isRunning:
		// Enabled and already up; converge its peer sets instead.
		network.reconcile()

	case record.Enabled && !isRunning:
		// Enabled and down; bring it up.
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
		record:          record,
		service:         r.service,
		clock:           r.clock,
		log:             r.log.With("network", record.Name),
		reconcileStatus: ActivityStatus{Interval: reconcileCap},
	}

	var mainHandler, inviteHandler http.Handler
	if r.peer != nil {
		mainHandler = r.peer.Router(record.Name)
	}
	if r.invite != nil {
		inviteHandler = r.invite.Router(record.Name)
	}

	if err := network.start(ctx, r.wireguard, mainHandler, inviteHandler); err != nil {
		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	r.running[record.Name] = network
	delete(r.reasons, record.Name)

	r.log.Info(
		"network started",
		"network",
		record.Name,
	)

	return nil
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

// getNetworkStatus reports the status of one network by name.
func (r *Runtime) getNetworkStatus(
	name string,
) (
	NetworkStatus,
	error,
) {
	network, err := r.service.GetNetwork(name)
	if err != nil {
		return NetworkStatus{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.parseStatusOf(network), nil
}

// parseStatusOf joins one persisted record with the running set. Callers
// hold r.mu.
func (r *Runtime) parseStatusOf(
	network *service.Network,
) NetworkStatus {
	running, isRunning := r.running[network.Name]
	status := NetworkStatus{
		Name:      network.Name,
		Enabled:   network.Enabled,
		Running:   isRunning,
		Reason:    r.reasons[network.Name],
		Reconcile: ActivityStatus{Interval: reconcileCap},
	}
	if isRunning {
		status.Reconcile, status.MainAPI, status.InviteAPI = running.getActivityStatuses()
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
		status.MainAPI,
		status.InviteAPI,
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

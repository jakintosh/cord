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
	"git.studiopollinator.com/pollinator/cord/internal/resource"
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

// Options configures the runtime.
type Options struct {
	// Service is the domain core holding the desired state.
	Service *service.Service

	// WireGuard is the manager used to create network devices.
	WireGuard *wireguard.Manager

	// DNS manages resolver configuration for permanent networks.
	DNS DNSManager

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
	dns        DNSManager
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

	reconcileStatus  resource.ActivityStatus
	cleanupStatus    resource.ActivityStatus
	dnsCleanupStatus resource.ActivityStatus
	dnsCleanupDone   bool
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
	if opts.DNS == nil {
		return nil, fmt.Errorf("client: DNS manager required")
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
		dns:            opts.DNS,
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

// ConvergeAll requests a full maintenance pass through the runtime owner.
func (r *Runtime) ConvergeAll(
	ctx context.Context,
) error {
	return execute(r, ctx, func() error {
		return r.convergeAll()
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

// convergeAll converges every installed network and stops anything still
// running that no longer has a record. Per-network failures are recorded
// and logged; only a failure to read the desired state is returned.
func (r *Runtime) convergeAll() (
	err error,
) {
	defer func() {
		r.reconcileStatus.Interval = r.interval
		r.reconcileStatus.Record(r.clock(), err)
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

	r.cleanupDNS(records)

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

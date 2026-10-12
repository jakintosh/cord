// Package runtime supervises the server networks a cord daemon is
// running. It is the only place a network is brought up: it reads the
// desired state the service persists, compares it against the networks
// actually running in this process, and reconciles the difference.
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

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultInterval is the interval between full maintenance passes, including
// retries for networks that failed to start.
const DefaultInterval = 30 * time.Second

var (
	// ErrNotRunning is returned once Run has stopped accepting commands.
	ErrNotRunning = errors.New("runtime is not running")
	// ErrAlreadyRun rejects concurrent runs and reuse of a finished runtime.
	ErrAlreadyRun = errors.New("runtime has already run")
)

// Handlers builds each plane's HTTP routes without starting resources.
type Handlers struct {
	Peer   func(string) http.Handler
	Invite func(string) http.Handler
}

// Options configures the runtime.
type Options struct {
	// Service is the domain core holding the desired state.
	Service *service.Service

	// WireGuard is the manager used to create network devices.
	WireGuard *wireguard.Manager

	// DNSPort is the port each network's authoritative DNS server
	// binds alongside its main plane. Zero requests an OS-assigned port.
	DNSPort uint16

	// DNSDisabled turns DNS serving off for every network. Zero value
	// keeps DNS on, which is production behavior.
	DNSDisabled bool

	// Interval is the period between full reconciliation passes. Defaults to
	// DefaultInterval when zero.
	Interval time.Duration

	// ShutdownTimeout bounds each HTTP or DNS server's graceful shutdown.
	// Non-positive values use the default of five seconds.
	ShutdownTimeout time.Duration

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives runtime diagnostics. Nil discards everything.
	Logger *slog.Logger
}

// Runtime owns its mutable state in one goroutine, executing submitted closures
// and maintenance sequentially. Public methods are safe for concurrent use;
// operations submitted before Run wait for it to start.
type Runtime struct {
	service         *service.Service
	wireguard       *wireguard.Manager
	handlers        Handlers
	dnsPort         uint16
	dnsDisabled     bool
	interval        time.Duration
	shutdownTimeout time.Duration
	clock           func() time.Time
	log             *slog.Logger

	operations chan operation
	done       chan struct{}
	started    atomic.Bool

	// Only Run accesses the fields below and the resources they reference.
	ctx             context.Context
	running         map[string]*Network
	reasons         map[string]string
	reconcileStatus resource.ActivityStatus
}

// New constructs a runtime without starting work. Call Run exactly once.
func New(
	opts Options,
) (
	*Runtime,
	error,
) {
	if opts.Service == nil {
		return nil, &resource.Error{
			Recovery: resource.Fatal,
			Cause:    fmt.Errorf("server: service required"),
		}
	}

	if opts.WireGuard == nil {
		return nil, &resource.Error{
			Recovery: resource.Fatal,
			Cause:    fmt.Errorf("server: wireguard manager required"),
		}
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

	r := &Runtime{
		service:         opts.Service,
		wireguard:       opts.WireGuard,
		dnsPort:         opts.DNSPort,
		dnsDisabled:     opts.DNSDisabled,
		interval:        interval,
		shutdownTimeout: opts.ShutdownTimeout,
		clock:           clock,
		log:             log,
		running:         make(map[string]*Network),
		operations:      make(chan operation),
		done:            make(chan struct{}),
		reasons:         make(map[string]string),
		reconcileStatus: resource.ActivityStatus{
			Interval: interval,
		},
	}

	return r, nil
}

// Run applies saved intent, executes operations, and maintains resources until
// cancellation. It closes resources before returning. A Runtime is single-use;
// construct another instance to restart it. Handlers must pass request contexts
// to runtime calls so closing a plane can release handlers waiting on this loop.
func (r *Runtime) Run(
	ctx context.Context,
	handlers Handlers,
) error {
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyRun
	}

	r.ctx = ctx
	r.handlers = handlers
	if ctx.Err() == nil {
		if err := r.reconcile(); err != nil {
			r.log.Error("runtime maintenance failed", "err", err)
		}
	}

	timer := time.NewTimer(r.interval)
	defer timer.Stop()

running:
	for {
		select {

		// This runtime has been cancelled, exit.
		case <-ctx.Done():
			break running

		// Process an incoming operation.
		case op := <-r.operations:
			if ctx.Err() != nil {
				// This runtime has been cancelled, exit.
				break running
			}

			if op.ctx.Err() != nil {
				// Operation caller canceled before execution.
				break
			}

			op.run()

		// Process a reconciliation timer.
		case <-timer.C:
			if ctx.Err() != nil {
				// this runtime has been cancelled, exit.
				break running
			}

			if err := r.reconcile(); err != nil {
				r.log.Error("runtime maintenance failed", "err", err)
			}

			// Re-arm the timer.
			timer.Reset(r.interval)
		}
	}

	// Release callers before draining HTTP: handlers may be waiting on us.
	// Plane closure also cancels its request contexts, including during a
	// network-disable command while the rest of the runtime stays active.
	close(r.done)

	var errs []error
	for name, network := range r.running {
		errs = append(errs, r.closeNetwork(name, network))
	}

	return errors.Join(errs...)
}

// reconcile gives every network a turn, and the full pass gets
// one activity result. Child failures are contained while this loop can retry.
func (r *Runtime) reconcile() (err error) {

	// Record one outcome for the entire pass, including early failures.
	defer func() {
		err = errors.Join(err, r.ctx.Err())
		r.reconcileStatus.Record(r.clock(), err)
	}()

	// First get all the networks tracked by the service.
	networks, err := r.service.ListNetworks()
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	// Then build a map of those known networks.
	known := make(map[string]bool, len(networks))
	for _, network := range networks {
		known[network.Name] = true
	}

	// Then close any running networks that are not known.
	var errs []error
	for name, network := range r.running {
		if known[name] {
			continue
		}

		if err := r.closeNetwork(name, network); err != nil {
			// Deleted records can still have resources after failed cleanup.
			errs = append(errs, err)
		}
	}

	// Attempt a reconcile of every saved network even if another network fails.
	for _, network := range networks {
		if r.ctx.Err() != nil {
			break // The pass outcome includes cancellation.
		}

		if err := r.reconcileNetwork(network); err != nil {
			errs = append(errs, fmt.Errorf("network %q: %w", network.Name, err))
		}
	}

	return errors.Join(errs...)
}

// operation carries a closure to the sole owner of runtime state.
type operation struct {
	ctx context.Context
	run func()
}

// executeValue submits an operation and returns its value through a buffered
// reply. execute uses the same submission path for error-only operations.
//
// Public methods submit closures; code already executing in the runtime goroutine
// must call service or private resource methods directly. Submitting another
// operation and waiting for it would deadlock.
//
// Operations called by plane handlers may update peers, but must not stop or
// replace their serving plane: draining HTTP would wait for the calling handler.
//
// Cancellation can release a waiting caller while its operation finishes.
// It does not undo an operation that has already begun.
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

	// Buffer the result so the runtime can finish if the caller stops waiting.
	reply := make(chan result, 1)

	// Build the operation that the runtime will execute.
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

	// Wait for the runtime to accept the operation.
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-r.done:
		return zero, ErrNotRunning
	case r.operations <- op:
		break // Accepted; now wait for its result.
	}

	// Wait for the result, caller cancellation, or runtime shutdown.
	select {
	case result := <-reply:
		return result.value, result.err
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-r.done:
		return zero, ErrNotRunning
	}
}

// execute submits an error-only operation through executeValue.
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

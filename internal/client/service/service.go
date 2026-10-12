package service

import (
	"fmt"
	"log/slog"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

// Options configures the domain core for the cord client daemon.
type Options struct {
	// Store is the persistence adapter for client-side network state.
	Store Store

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service. Nil
	// discards everything.
	Logger *slog.Logger
}

// Service is the domain core for the cord client daemon. All domain
// operations are methods on Service, scoped by a network name
// parameter. It owns durable state through the Store and validates
// everything that becomes a membership record, but never touches
// devices, network clients, goroutines, or timers.
type Service struct {
	store Store
	clock func() time.Time
	log   *slog.Logger
}

// New returns a ready-to-use Service.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.Store == nil {
		return nil, fmt.Errorf("client: store required")
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}

	return &Service{
		store: opts.Store,
		clock: clock,
		log:   log,
	}, nil
}

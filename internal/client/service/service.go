package service

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Options configures the domain core for the cord client daemon.
type Options struct {
	// Store is the persistence adapter for client-side network state.
	Store Store

	// WG is the WireGuard manager for creating network devices.
	WG wireguard.WG

	// Clock returns the current time. Defaults to time.Now when nil.
	Clock func() time.Time

	// Logger receives internal diagnostics from the service.
	Logger *log.Logger

	// SyncInterval controls how often the sync loop runs for each
	// enabled network. Defaults to 30s when zero.
	SyncInterval time.Duration

	// ScanInterval controls how often the peer scan loop runs for
	// each enabled network. Defaults to 5m when zero.
	ScanInterval time.Duration

	// HTTPClient is the HTTP client used to reach the server's peer
	// and invite APIs through the WireGuard tunnel. Nil uses a
	// default client with a 10s timeout.
	HTTPClient *http.Client
}

// Service is the domain core for the cord client daemon. It manages
// client-side network memberships, their WireGuard interfaces, and
// background peer synchronization. All domain operations are methods on
// Service, scoped by a network name parameter.
//
// The service owns durable state through the Store and live WireGuard
// state through the WG abstraction.
type Service struct {
	store        Store
	wg           wireguard.WG
	clock        func() time.Time
	log          *log.Logger
	httpClient   *http.Client
	mu           sync.Mutex
	syncInterval time.Duration
	scanInterval time.Duration

	// running tracks networks that are currently enabled (interface
	// up, sync loop active). The key is the network name.
	running map[string]*LiveNetwork
}

// LiveNetwork holds the live resources for one enabled client network:
// the WireGuard device, the server API client, and sync status.
type LiveNetwork struct {
	Device    wireguard.WGDevice
	ApiClient *serverapi.Client
	Cancel    context.CancelFunc
	LastSync  time.Time
	LastErr   string
}

// New returns a ready-to-use Service. Store and WG must be non-nil.
func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.Store == nil {
		return nil, errors.New("service: Store is required")
	}
	if opts.WG == nil {
		return nil, errors.New("service: WG is required")
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	syncInterval := opts.SyncInterval
	if syncInterval == 0 {
		syncInterval = SyncInterval
	}
	scanInterval := opts.ScanInterval
	if scanInterval == 0 {
		scanInterval = ScanInterval
	}
	return &Service{
		store:        opts.Store,
		wg:           opts.WG,
		clock:        opts.Clock,
		log:          opts.Logger,
		httpClient:   opts.HTTPClient,
		running:      make(map[string]*LiveNetwork),
		syncInterval: syncInterval,
		scanInterval: scanInterval,
	}, nil
}

// Start is called once at daemon startup. It loads every network with
// enabled=true from the store and enables each one (brings up
// interfaces, starts sync loops). Networks that fail to start are
// logged and left disabled.
func (s *Service) Start(
	ctx context.Context,
) error {
	names, err := s.store.ListNetworkNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		network, err := s.store.GetNetwork(name)
		if err != nil {
			s.logf("start: get network %q: %v", name, err)
			continue
		}
		if !network.Enabled {
			continue
		}

		if err := s.EnableNetwork(ctx, name); err != nil {
			s.logf("start: enable network %q: %v", name, err)
		}
	}

	return nil
}

// Close shuts down all running networks (disables each one) and
// releases resources. It should be called during daemon shutdown.
func (s *Service) Close() error {
	s.mu.Lock()
	running := s.running
	s.running = make(map[string]*LiveNetwork)
	s.mu.Unlock()

	for name, ln := range running {
		s.stopLive(name, ln)
	}
	return nil
}

// logf writes a message to the service logger if configured.
func (s *Service) logf(format string, args ...any) {
	if s.log != nil {
		s.log.Printf(format, args...)
	}
}

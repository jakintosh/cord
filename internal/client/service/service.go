package service

import (
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
	Store     Store
	WireGuard *wireguard.Manager
	Clock     func() time.Time
	Logger    *log.Logger

	// Overrides for tests only — defaults are package consts.
	SyncInterval   time.Duration
	ScanInterval   time.Duration
	ReportInterval time.Duration

	HTTPClient *http.Client
}

// Service is the domain core for the cord client daemon. It manages
// client-side network memberships, their WireGuard interfaces, and
// background peer synchronization. All domain operations are methods on
// Service, scoped by a network name parameter.
type Service struct {
	store      Store
	wireguard  *wireguard.Manager
	clock      func() time.Time
	log        *log.Logger
	httpClient *http.Client

	syncInterval   time.Duration
	scanInterval   time.Duration
	reportInterval time.Duration

	mu      sync.Mutex
	running map[string]*Network
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
	if opts.WireGuard == nil {
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
	reportInterval := opts.ReportInterval
	if reportInterval == 0 {
		reportInterval = ReportInterval
	}
	return &Service{
		store:          opts.Store,
		wireguard:      opts.WireGuard,
		clock:          opts.Clock,
		log:            opts.Logger,
		httpClient:     opts.HTTPClient,
		running:        make(map[string]*Network),
		syncInterval:   syncInterval,
		scanInterval:   scanInterval,
		reportInterval: reportInterval,
	}, nil
}

// Start brings up every network with enabled=true. Networks that fail
// to start are logged and left disabled. No context is needed — the
// Service owns its own start/shutdown lifecycle.
func (s *Service) Start() error {
	names, err := s.store.ListNetworkNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		nc, err := s.store.GetNetwork(name)
		if err != nil {
			s.logf("start: get network %q: %v", name, err)
			continue
		}
		if !nc.Enabled {
			continue
		}

		if err := s.EnableNetwork(name); err != nil {
			s.logf("start: enable network %q: %v", name, err)
		}
	}

	return nil
}

// Close shuts down all running networks and releases resources.
func (s *Service) Close() error {
	s.mu.Lock()
	running := s.running
	s.running = make(map[string]*Network)
	s.mu.Unlock()

	for name, n := range running {
		if err := n.stop(); err != nil {
			s.logf("close: stop network %q: %v", name, err)
		}
	}
	return nil
}

func (s *Service) newInviteClient(
	tunnel *Tunnel,
) *serverapi.InviteClient {
	return serverapi.NewInviteClient(tunnel.apiAddr, s.httpClient)
}

func (s *Service) newPeerClient(
	tunnel *Tunnel,
) *serverapi.PeerClient {
	return serverapi.NewPeerClient(tunnel.apiAddr, s.httpClient)
}

func (s *Service) logf(
	format string,
	args ...any,
) {
	if s.log != nil {
		s.log.Printf(format, args...)
	}
}

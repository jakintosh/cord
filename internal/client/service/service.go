package service

import (
	"context"
	"log"
	"sync"
	"time"
)

type Options struct {
	Store  Store
	WG     WG
	Clock  func() time.Time
	Logger *log.Logger
}

type Service struct {
	store   Store
	wg      WG
	clock   func() time.Time
	log     *log.Logger
	mu      sync.Mutex
	running map[string]*runningNetwork
}

type runningNetwork struct {
	device WGDevice
	cancel context.CancelFunc
}

func New(
	opts Options,
) (
	*Service,
	error,
) {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	return &Service{
		store:   opts.Store,
		wg:      opts.WG,
		clock:   opts.Clock,
		log:     opts.Logger,
		running: make(map[string]*runningNetwork),
	}, nil
}

func (s *Service) Start(
	ctx context.Context,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	if s.wg == nil {
		return ErrNotImplemented
	}

	names, err := s.store.ListNetworkNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		nw, err := s.store.GetNetwork(name)
		if err != nil {
			if s.log != nil {
				s.log.Printf("start: get network %q: %v", name, err)
			}
			continue
		}
		if !nw.Enabled {
			continue
		}

		if err := s.EnableNetwork(ctx, name); err != nil {
			if s.log != nil {
				s.log.Printf("start: enable network %q: %v", name, err)
			}
			_, _ = s.store.UpdateNetwork(name, UpdateNetworkRequest{
				Enabled: &[]bool{false}[0],
			})
		}
	}

	return nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name := range s.running {
		s.disableLocked(name)
	}
	return nil
}

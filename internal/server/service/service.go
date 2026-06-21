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
	main   WGDevice
	invite WGDevice
	cancel context.CancelFunc
}

func New(
	opts Options,
) (
	*Service,
	error,
) {
	return &Service{
		store:   opts.Store,
		wg:      opts.WG,
		clock:   opts.Clock,
		log:     opts.Logger,
		running: make(map[string]*runningNetwork),
	}, nil
}

func (s *Service) Close() error {
	return nil
}

func (s *Service) StartNetwork(
	ctx context.Context,
	name string,
) error {
	return ErrNotImplemented
}

func (s *Service) StopNetwork(
	name string,
) error {
	return ErrNotImplemented
}

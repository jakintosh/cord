package service

import (
	"context"
	"time"
)

type Network struct {
	Name           string
	PrivateKey     string
	PublicKey      string
	AssignedCidr   string
	ServerPubkey   string
	ServerEndpoint string
	ServerApiAddr  string
	Enabled        bool
	CreatedAt      time.Time
}

type NetworkStatus struct {
	Name      string
	Enabled   bool
	Running   bool
	Degraded  bool
	LastSync  time.Time
	LastError string
	PeerCount int
}

type UpdateNetworkRequest struct {
	Enabled *bool
}

func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.GetNetwork(name)
}

func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.ListNetworkNames()
}

func (s *Service) ShowNetwork(
	name string,
) (
	*Network,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.GetNetwork(name)
}

func (s *Service) Status() (
	[]NetworkStatus,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}

	names, err := s.store.ListNetworkNames()
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	running := make(map[string]bool, len(s.running))
	for name := range s.running {
		running[name] = true
	}
	s.mu.Unlock()

	statuses := make([]NetworkStatus, 0, len(names))
	for _, name := range names {
		statuses = append(statuses, NetworkStatus{
			Name:    name,
			Running: running[name],
		})
	}
	return statuses, nil
}

func (s *Service) InstallNetwork(
	invitePath string,
) (
	*Network,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	if s.wg == nil {
		return nil, ErrNotImplemented
	}
	return nil, ErrNotImplemented
}

func (s *Service) UninstallNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}

	_ = s.DisableNetwork(name)

	return s.store.DeleteNetwork(name)
}

func (s *Service) EnableNetwork(
	ctx context.Context,
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	if s.wg == nil {
		return ErrNotImplemented
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.running[name]; ok {
		return nil
	}

	nw, err := s.store.GetNetwork(name)
	if err != nil {
		return err
	}
	if nw.Enabled {
		return nil
	}

	if err := s.enableLocked(ctx, nw); err != nil {
		return err
	}

	yes := true
	_, err = s.store.UpdateNetwork(name, UpdateNetworkRequest{Enabled: &yes})
	if err != nil {
		s.disableLocked(name)
		return err
	}
	return nil
}

func (s *Service) DisableNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}

	s.mu.Lock()
	s.disableLocked(name)
	s.mu.Unlock()

	if s.wg == nil {
		return nil
	}

	no := false
	_, err := s.store.UpdateNetwork(name, UpdateNetworkRequest{Enabled: &no})
	return err
}

func (s *Service) FetchNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	return ErrNotImplemented
}

func (s *Service) enableLocked(
	ctx context.Context,
	nw *Network,
) error {
	device, err := s.wg.NewDevice(
		nw.Name,
		nw.PrivateKey,
		nw.AssignedCidr,
		0,
	)
	if err != nil {
		return err
	}

	peers, err := s.store.ListPeers(nw.Name)
	if err != nil {
		_ = device.Down(true)
		return err
	}

	if err := device.ApplyPeers(s.buildPeers(nw, peers)); err != nil {
		_ = device.Down(true)
		return err
	}

	if err := device.Up(); err != nil {
		_ = device.Down(true)
		return err
	}

	syncCtx, cancel := context.WithCancel(context.Background())
	go s.syncLoop(syncCtx, nw.Name)

	s.running[nw.Name] = &runningNetwork{
		device: device,
		cancel: cancel,
	}
	return nil
}

func (s *Service) disableLocked(
	name string,
) {
	rn, ok := s.running[name]
	if !ok {
		return
	}

	rn.cancel()
	_ = rn.device.Down(true)
	delete(s.running, name)
}

func (s *Service) syncLoop(
	ctx context.Context,
	name string,
) {
	<-ctx.Done()
}

func (s *Service) buildPeers(
	nw *Network,
	peers []*Peer,
) []WGPeer {
	wgPeers := make([]WGPeer, 0, len(peers)+1)

	wgPeers = append(wgPeers, WGPeer{
		PublicKey:  nw.ServerPubkey,
		AllowedIPs: []string{nw.AssignedCidr},
		Endpoint:   nw.ServerEndpoint,
	})

	for _, p := range peers {
		wgPeers = append(wgPeers, WGPeer{
			PublicKey:  p.PublicKey,
			AllowedIPs: []string{p.Cidr},
			Endpoint:   p.Endpoint,
		})
	}

	return wgPeers
}

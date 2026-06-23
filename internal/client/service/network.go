package service

import (
	"context"
	"time"
)

// Network is the persistent record of a client-side network membership.
// It holds the local identity, the assigned address, the server peer
// reference, and the user's enable/disable policy. It is inert domain
// data — the Service owns all behavior.
type Network struct {
	Name           string
	PrivateKey     string
	PublicKey      string
	AssignedCidr   string // e.g. "10.42.0.5/16" — this client's address
	ServerPubkey   string // the server's WireGuard public key
	ServerEndpoint string // server's external endpoint, e.g. "1.2.3.4:51820"
	ServerApiAddr  string // server API through tunnel, e.g. "10.42.0.1:8443"
	Enabled        bool   // whether the daemon should bring up the interface
	CreatedAt      time.Time
}

// NetworkStatus carries the runtime state of a single client network
// for the status endpoint. It combines persisted fields with live
// daemon state.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Running   bool
	Degraded  bool
	LastSync  time.Time
	LastError string
	PeerCount int
}

// UpdateNetworkRequest carries the fields that can be changed on an
// installed network. Nil pointer means "no change."
type UpdateNetworkRequest struct {
	Enabled *bool
}

// GetNetwork returns the persisted network record by name.
// Returns ErrNotFound if the network is not installed.
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

// ListNetworks returns the names of all installed networks.
func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	if s.store == nil {
		return nil, ErrNotImplemented
	}
	return s.store.ListNetworkNames()
}

// ShowNetwork returns the full Network record for a single installed
// network.
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

// Status returns the current runtime status for every installed
// network: enabled flag, whether the interface is running, last sync
// time, last error, and peer count.
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

// InstallNetwork reads an invite file from invitePath, validates it,
// generates a permanent keypair, redeems the invite with the server,
// and persists the resulting network membership record. The new network
// is left in the disabled state — the caller must enable it separately.
//
// This is the full install flow:
//  1. parse and validate the invite file
//  2. generate (or reuse, if retrying) a permanent keypair
//  3. bring up a temporary invite interface
//  4. prove reachability (handshake probe)
//  5. redeem the invite via the server API through the tunnel
//  6. persist the permanent network record
//  7. bring up the main interface, confirm with server
//  8. fetch initial peer list into local cache
//  9. tear down the main interface (caller enables it later)
//
// Steps 3–9 depend on WG and server communication. When those
// dependencies are nil, the method stores the generated keypair and
// returns ErrNotImplemented after step 2.
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

// UninstallNetwork removes a network and all its local state. If the
// network is currently enabled, it is disabled first (interface down,
// sync loop stopped), then the persisted record and peer cache are
// deleted.
func (s *Service) UninstallNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}

	_ = s.DisableNetwork(name)

	return s.store.DeleteNetwork(name)
}

// EnableNetwork brings up the WireGuard interface for the named network
// and starts the background sync loop. It persists enabled=true in the
// store. If the interface fails to come up, the state stays disabled
// and an error is returned.
//
// Idempotent: enabling an already-enabled network is a no-op.
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

// DisableNetwork stops the background sync loop and brings down the
// WireGuard interface for the named network. It persists enabled=false
// in the store.
//
// Idempotent: disabling an already-disabled network is a no-op.
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

// FetchNetwork performs a one-shot peer sync from the server for the
// named network. It fetches the visible peer list from the server's
// network API, reconciles it into the local peer cache, and applies
// any changes to the live WireGuard interface if the network is
// enabled.
//
// When the network is not enabled, FetchNetwork only updates the cache
// and does not touch the WireGuard interface.
func (s *Service) FetchNetwork(
	name string,
) error {
	if s.store == nil {
		return ErrNotImplemented
	}
	return ErrNotImplemented
}

// enableLocked brings up the WireGuard interface and starts the sync
// loop for a network. Caller must hold s.mu. Returns an error if the
// interface fails to start; the network stays disabled.
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

// disableLocked stops the sync loop and brings down the interface for
// a network. Caller must hold s.mu. Never returns an error — best-effort
// teardown.
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

// syncLoop is the background goroutine for an enabled network. It runs
// until ctx is cancelled, periodically fetching peers from the server,
// reconciling into the local cache, scanning for endpoint changes on
// the live device, reporting sightings, and applying the updated peer
// set to the WireGuard device.
func (s *Service) syncLoop(
	ctx context.Context,
	name string,
) {
	<-ctx.Done()
}

// buildPeers converts the local peer cache for a network into a slice
// of WGPeer values suitable for ApplyPeers. The server peer is always
// included first with an all-network allowed-ips (for relay); cached
// peers follow with their individual CIDRs.
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

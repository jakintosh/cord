package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

const (
	syncInterval     = 10 * time.Second
	shutdownTimeout  = 5 * time.Second
	maintenanceEvery = 6 // sync ticks between maintenance passes
)

// Runtime owns the live state of a serving cord network: the main and
// invite WireGuard interfaces and the listeners for the HTTP API.
type Runtime struct {
	Srv    *Server
	Cfg    *NetworkConfig
	Notify chan struct{} // poke to trigger an immediate peer sync

	main   *wg.Interface
	invite *wg.Interface
}

// NewRuntime loads the network config and prepares (but does not bring
// up) both WireGuard interfaces.
func NewRuntime(
	srv *Server,
	noRouting bool,
	mtu int,
	backend wg.BackendType,
) (
	*Runtime,
	error,
) {
	cfg, err := srv.LoadConfig()
	if err != nil {
		return nil, err
	}

	privKey, err := wg.ParseKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key in config: %w", err)
	}

	rootNet, err := cfg.RootNet()
	if err != nil {
		return nil, err
	}
	serverIP, err := cfg.ServerIP()
	if err != nil {
		return nil, err
	}

	inviteNet, err := cfg.InviteNet()
	if err != nil {
		return nil, err
	}
	inviteIP, err := cfg.InviteServerIP()
	if err != nil {
		return nil, err
	}

	mainName, inviteName, err := wg.NetworkInterfaceNames(srv.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid network interface names: %w", err)
	}

	main, err := wg.NewInterface(
		mainName,
		privKey,
		net.IPNet{IP: serverIP, Mask: rootNet.Mask},
		int(cfg.ListenPort),
		backend,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create main interface: %w", err)
	}
	main.MTU = mtu
	main.NoRoutes = noRouting

	invite, err := wg.NewInterface(
		inviteName,
		privKey,
		net.IPNet{IP: inviteIP, Mask: inviteNet.Mask},
		int(cfg.InviteListenPort),
		backend,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create invite interface: %w", err)
	}
	invite.MTU = mtu
	invite.NoRoutes = noRouting

	return &Runtime{
		Srv:    srv,
		Cfg:    cfg,
		Notify: make(chan struct{}, 1),
		main:   main,
		invite: invite,
	}, nil
}

// Poke requests an immediate interface sync; safe to call from any
// goroutine and never blocks.
func (r *Runtime) Poke() {
	select {
	case r.Notify <- struct{}{}:
	default:
	}
}

// SyncPeers rebuilds both interfaces' peer lists from the database and
// applies them to the live devices.
func (r *Runtime) SyncPeers() error {
	mainPeers, err := r.mainPeers()
	if err != nil {
		return fmt.Errorf("failed to build main peers: %w", err)
	}
	r.main.SetPeers(mainPeers)
	if err := r.main.Sync(); err != nil {
		return fmt.Errorf("failed to sync main interface: %w", err)
	}

	invitePeers, err := r.invitePeers()
	if err != nil {
		return fmt.Errorf("failed to build invite peers: %w", err)
	}
	r.invite.SetPeers(invitePeers)
	if err := r.invite.Sync(); err != nil {
		return fmt.Errorf("failed to sync invite interface: %w", err)
	}

	return nil
}

// mainPeers converts confirmed, enabled peers into WireGuard peers for
// the main interface. The server's own record is excluded.
func (r *Runtime) mainPeers() ([]wg.Peer, error) {
	peers, err := r.Srv.Store.PeerList()
	if err != nil {
		return nil, err
	}

	wgPeers := make([]wg.Peer, 0, len(peers))
	for _, peer := range peers {
		if !peer.Confirmed || !peer.Enabled {
			continue
		}
		if peer.PublicKey == r.Cfg.PublicKey {
			continue
		}
		wgPeer, err := peerFromRecord(peer.PublicKey, peer.Cidr)
		if err != nil {
			log.Printf("skipping peer '%s': %v", peer.Name, err)
			continue
		}
		wgPeers = append(wgPeers, *wgPeer)
	}

	return wgPeers, nil
}

// invitePeers converts active invites into WireGuard peers for the
// invite interface.
func (r *Runtime) invitePeers() ([]wg.Peer, error) {
	invites, err := r.Srv.Store.InviteListActive()
	if err != nil {
		return nil, err
	}

	wgPeers := make([]wg.Peer, 0, len(invites))
	for _, invite := range invites {
		wgPeer, err := peerFromRecord(invite.PublicKey, invite.InviteCidr)
		if err != nil {
			log.Printf("skipping invite '%s': %v", invite.Name, err)
			continue
		}
		wgPeers = append(wgPeers, *wgPeer)
	}

	return wgPeers, nil
}

func peerFromRecord(pubKey string, cidr string) (*wg.Peer, error) {
	key, err := wg.ParseKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	_, allowed, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid cidr '%s': %w", cidr, err)
	}
	return &wg.Peer{
		PublicKey:  key,
		AllowedIPs: []net.IPNet{*allowed},
	}, nil
}

// Run brings up both interfaces, serves the HTTP APIs on them, and
// keeps interface peer lists in sync with the database until the
// context is cancelled. The invite handler should only expose the
// redemption endpoint (see ADR-001).
func (r *Runtime) Run(
	ctx context.Context,
	mainHandler http.Handler,
	inviteHandler http.Handler,
) error {
	// Bring up the interfaces
	if err := r.main.Up(""); err != nil {
		return fmt.Errorf("failed to bring up main interface: %w", err)
	}
	defer func() { _ = r.main.Down(true) }()
	log.Printf("main interface up: %s (%s)", r.main.DeviceName(), r.main.Address.String())

	if err := r.invite.Up(""); err != nil {
		return fmt.Errorf("failed to bring up invite interface: %w", err)
	}
	defer func() { _ = r.invite.Down(true) }()
	log.Printf("invite interface up: %s (%s)", r.invite.DeviceName(), r.invite.Address.String())

	// Apply the initial peer lists
	if err := r.SyncPeers(); err != nil {
		return err
	}

	// Start the HTTP APIs on the internal addresses
	mainAddr, err := r.Cfg.InternalApiEndpoint()
	if err != nil {
		return err
	}
	inviteAddr, err := r.Cfg.InviteApiEndpoint()
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	mainSrv := &http.Server{Addr: mainAddr, Handler: mainHandler}
	inviteSrv := &http.Server{Addr: inviteAddr, Handler: inviteHandler}
	go serveHTTP("main api", mainSrv, errCh)
	go serveHTTP("invite api", inviteSrv, errCh)
	defer shutdownHTTP(mainSrv, inviteSrv)

	// Sync loop: periodic + poked, with occasional maintenance
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	ticksSinceMaintenance := 0

	for {
		select {
		case <-ctx.Done():
			log.Printf("shutting down")
			return nil

		case err := <-errCh:
			return err

		case <-ticker.C:
			ticksSinceMaintenance++
			if ticksSinceMaintenance >= maintenanceEvery {
				ticksSinceMaintenance = 0
				r.maintain()
			}
			if err := r.SyncPeers(); err != nil {
				log.Printf("sync failed: %v", err)
			}

		case <-r.Notify:
			if err := r.SyncPeers(); err != nil {
				log.Printf("sync failed: %v", err)
			}
		}
	}
}

// maintain prunes expired invites and stale endpoint sightings.
func (r *Runtime) maintain() {
	now := time.Now()
	if err := r.Srv.Store.InvitesPruneExpired(now.Unix()); err != nil {
		log.Printf("invite pruning failed: %v", err)
	}
	if err := r.Srv.Store.EndpointsPrune(now.Add(-endpointTTL).Unix()); err != nil {
		log.Printf("endpoint pruning failed: %v", err)
	}
}

func serveHTTP(name string, srv *http.Server, errCh chan<- error) {
	log.Printf("%s listening on %s", name, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- fmt.Errorf("%s server failed: %w", name, err)
	}
}

func shutdownHTTP(servers ...*http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(ctx)
	}
}

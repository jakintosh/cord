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

// Service owns the live state of a serving cord network: the main and
// invite WireGuard interfaces and the listeners for the HTTP API.
type Service struct {
	Srv     *Server
	Network *Network
	Notify  chan struct{} // poke to trigger an immediate peer reconciliation

	main   *wg.Interface
	invite *wg.Interface
}

// NewService loads the network config and prepares (but does not bring
// up) both WireGuard interfaces.
func NewService(
	srv *Server,
	noRouting bool,
	mtu int,
	backend wg.BackendType,
	verbose bool,
) (
	*Service,
	error,
) {
	cfg, err := srv.LoadNetwork()
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
	if verbose {
		logf := func(format string, args ...any) {
			log.Printf("verbose: "+format, args...)
		}
		main.SetReconcileLogger(logf)
		invite.SetReconcileLogger(logf)
	}

	return &Service{
		Srv:     srv,
		Network: cfg,
		Notify:  make(chan struct{}, 1),
		main:    main,
		invite:  invite,
	}, nil
}

// Poke requests an immediate interface reconciliation; safe to call from any
// goroutine and never blocks.
func (s *Service) Poke() {
	select {
	case s.Notify <- struct{}{}:
	default:
	}
}

// ReconcilePeers rebuilds both interfaces' desired peer lists from the database and
// reconciles them against the live WireGuard devices.
func (s *Service) ReconcilePeers() error {
	var reconcileErrors []error

	mainPeers, err := s.mainPeers()
	if err != nil {
		reconcileErrors = append(reconcileErrors, fmt.Errorf("failed to build main peers: %w", err))
	} else {
		s.main.SetPeers(mainPeers)
		if err := s.main.Reconcile(); err != nil {
			reconcileErrors = append(
				reconcileErrors,
				fmt.Errorf("failed to reconcile main interface: %w", err),
			)
		}
	}

	invitePeers, err := s.invitePeers()
	if err != nil {
		reconcileErrors = append(reconcileErrors, fmt.Errorf("failed to build invite peers: %w", err))
	} else {
		s.invite.SetPeers(invitePeers)
		if err := s.invite.Reconcile(); err != nil {
			reconcileErrors = append(
				reconcileErrors,
				fmt.Errorf("failed to reconcile invite interface: %w", err),
			)
		}
	}

	return errors.Join(reconcileErrors...)
}

// ReconcileStatus returns structured status for the main and invite devices.
// Failed plans remain pending until a later reconciliation re-plans and applies.
func (s *Service) ReconcileStatus() (wg.ReconcileStatus, wg.ReconcileStatus) {
	return s.main.ReconcileStatus(), s.invite.ReconcileStatus()
}

// mainPeers converts enabled peers into WireGuard peers for the main
// interface. Redeemed-but-unconfirmed peers must be present so they can
// reach the confirmation endpoint; normal API routes still reject them.
// The server's own record is excluded.
func (s *Service) mainPeers() ([]wg.Peer, error) {
	peers, err := s.Srv.Store.PeerList()
	if err != nil {
		return nil, err
	}

	return mainPeersFromRecords(peers, s.Network.PublicKey), nil
}

func mainPeersFromRecords(peers []*Peer, serverPublicKey string) []wg.Peer {
	wgPeers := make([]wg.Peer, 0, len(peers))
	for _, peer := range peers {
		if !peer.Enabled {
			continue
		}
		if peer.PublicKey == serverPublicKey {
			continue
		}
		wgPeer, err := peerFromRecord(peer.PublicKey, peer.Cidr)
		if err != nil {
			log.Printf("skipping peer '%s': %v", peer.Name, err)
			continue
		}
		wgPeers = append(wgPeers, *wgPeer)
	}

	return wgPeers
}

// invitePeers converts active invites into WireGuard peers for the
// invite interface.
func (s *Service) invitePeers() ([]wg.Peer, error) {
	invites, err := s.Srv.Store.InviteListActive()
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
		PublicKey:      key,
		AllowedIPs:     []net.IPNet{*allowed},
		EndpointPolicy: wg.EndpointDynamic,
	}, nil
}

// Run brings up both interfaces, serves the HTTP APIs on them, and
// keeps interface peer lists in sync with the database until the
// context is cancelled. The invite handler should only expose the
// redemption endpoint (see ADR-001).
func (s *Service) Run(
	ctx context.Context,
	mainHandler http.Handler,
	inviteHandler http.Handler,
) error {
	mainPeers, err := s.mainPeers()
	if err != nil {
		return fmt.Errorf("failed to build initial main peers: %w", err)
	}
	invitePeers, err := s.invitePeers()
	if err != nil {
		return fmt.Errorf("failed to build initial invite peers: %w", err)
	}
	s.main.SetPeers(mainPeers)
	s.invite.SetPeers(invitePeers)

	// Bring up the interfaces
	if err := s.main.Up(""); err != nil {
		return fmt.Errorf("failed to bring up main interface: %w", err)
	}
	defer func() { _ = s.main.Down(true) }()
	log.Printf("main interface up: %s (%s)", s.main.DeviceName(), s.main.Address.String())

	if err := s.invite.Up(""); err != nil {
		return fmt.Errorf("failed to bring up invite interface: %w", err)
	}
	defer func() { _ = s.invite.Down(true) }()
	log.Printf("invite interface up: %s (%s)", s.invite.DeviceName(), s.invite.Address.String())

	// Start the HTTP APIs on the internal addresses
	mainAddr, err := s.Network.InternalApiEndpoint()
	if err != nil {
		return err
	}
	inviteAddr, err := s.Network.InviteApiEndpoint()
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	mainSrv := &http.Server{Addr: mainAddr, Handler: mainHandler}
	inviteSrv := &http.Server{Addr: inviteAddr, Handler: inviteHandler}
	go serveHTTP("main api", mainSrv, errCh)
	go serveHTTP("invite api", inviteSrv, errCh)
	defer shutdownHTTP(mainSrv, inviteSrv)

	// Reconciliation loop: periodic + poked, with occasional maintenance
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
				s.maintain()
			}
			if err := s.ReconcilePeers(); err != nil {
				log.Printf("reconciliation failed: %v", err)
			}

		case <-s.Notify:
			if err := s.ReconcilePeers(); err != nil {
				log.Printf("reconciliation failed: %v", err)
			}
		}
	}
}

// maintain prunes expired invites and stale endpoint sightings.
func (s *Service) maintain() {
	now := time.Now()
	if err := s.Srv.Store.InvitesPruneExpired(now.Unix()); err != nil {
		log.Printf("invite pruning failed: %v", err)
	}
	if err := s.Srv.Store.EndpointsPrune(now.Add(-endpointTTL).Unix()); err != nil {
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

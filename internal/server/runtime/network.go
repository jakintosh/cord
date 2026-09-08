package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// NetworkReconcileInterval caps the time between network maintenance passes.
// Registration expiry can schedule an earlier pass.
const NetworkReconcileInterval = 5 * time.Minute

type networkOptions struct {
	record        *service.Network
	service       *service.Service
	wireguard     *wireguard.Manager
	mainHandler   http.Handler
	inviteHandler http.Handler
	dnsPort       uint16
	dnsDisabled   bool
	clock         func() time.Time
	log           *slog.Logger
}

// NetworkStatus combines current child resource state with the latest
// reconciliation activity. Reading it never performs maintenance.
type NetworkStatus struct {
	Main      PlaneStatus
	Invite    PlaneStatus
	Reconcile ActivityStatus
}

// Network owns two persistent planes and schedules their maintenance.
// Its mutex serializes snapshots, reconciliation, and shutdown.
type Network struct {
	record  *service.Network
	main    *Plane
	invite  *Plane
	service *service.Service
	clock   func() time.Time
	log     *slog.Logger

	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	timer           *time.Timer
	reconcileStatus ActivityStatus
}

// newNetwork constructs both planes without starting resources or scheduling work.
func newNetwork(
	opts networkOptions,
) (
	*Network,
	error,
) {
	var dnsOptions *dnsConfig
	if !opts.dnsDisabled {
		dnsOptions = &dnsConfig{
			network: opts.record.Name,
			service: opts.service,
			port:    opts.dnsPort,
		}
	}

	main, err := newPlane(
		opts.record.Main,
		dnsOptions,
		opts.record.PrivateKey,
		opts.mainHandler,
		opts.wireguard,
		opts.log.With("plane", "main"),
	)
	if err != nil {
		return nil, fmt.Errorf("main plane: %w", err)
	}

	invite, err := newPlane(
		opts.record.Invite,
		nil,
		opts.record.PrivateKey,
		opts.inviteHandler,
		opts.wireguard,
		opts.log.With("plane", "invite"),
	)
	if err != nil {
		return nil, fmt.Errorf("invite plane: %w", err)
	}

	return &Network{
		record:  opts.record,
		main:    main,
		invite:  invite,
		service: opts.service,
		clock:   opts.clock,
		log:     opts.log,
		reconcileStatus: ActivityStatus{
			Interval: NetworkReconcileInterval,
		},
	}, nil
}

// Status collects plane snapshots and activity without changing resources.
func (n *Network) Status() NetworkStatus {
	n.mu.Lock()
	defer n.mu.Unlock()

	return NetworkStatus{
		Main:      n.main.Status(),
		Invite:    n.invite.Status(),
		Reconcile: n.reconcileStatus,
	}
}

// Reconcile maintains the network and schedules the next pass. The first call
// after construction or Stop establishes a lifetime derived from ctx.
func (n *Network) Reconcile(
	ctx context.Context,
) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Honor caller cancellation before doing any lifecycle work.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Clean up a canceled lifetime before starting a new one.
	if n.ctx != nil && n.ctx.Err() != nil {
		if err := n.stopLocked(); err != nil {
			return err
		}
	}

	// Share one context across passes, with cancellation owned by Stop.
	if n.ctx == nil {
		n.ctx, n.cancel = context.WithCancel(ctx)
	}

	return n.reconcileLocked()
}

// Stop cancels scheduled work and stops both planes, retaining their wrappers.
// A subsequent explicit Reconcile can establish a new lifetime.
func (n *Network) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.stopLocked()
}

// reconcileLocked requires mu and completes one maintenance pass.
func (n *Network) reconcileLocked() error {
	now := n.clock()
	var errs []error

	if err := n.main.Reconcile(n.ctx); err != nil {
		errs = append(errs, fmt.Errorf("main plane: %w", err))
	}
	if err := n.invite.Reconcile(n.ctx); err != nil {
		errs = append(errs, fmt.Errorf("invite plane: %w", err))
	}

	expiry, peerErr := n.reconcilePeersLocked(now)
	errs = append(errs, peerErr)

	err := errors.Join(errs...)
	n.reconcileStatus.record(n.clock(), err)

	n.rearmLocked(now, expiry)
	return err
}

// stopLocked requires mu. Invalidating the context also retires callbacks
// already queued by the old timer; they will do nothing after acquiring mu.
func (n *Network) stopLocked() error {

	// Cancel this lifetime so children and queued callbacks see shutdown.
	if n.cancel != nil {
		n.cancel()
	}

	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}

	// Forget this lifetime so an explicit Reconcile can start another.
	n.ctx, n.cancel = nil, nil

	return errors.Join(
		n.main.Stop(),
		n.invite.Stop(),
	)
}

// reconcilePeersLocked requires mu. Pruning must succeed before applying peer
// state. After that, a failure on one plane does not prevent updating the other.
func (n *Network) reconcilePeersLocked(
	now time.Time,
) (
	time.Time,
	error,
) {
	name := n.record.Name
	if err := n.service.PruneNetwork(name); err != nil {
		return time.Time{}, fmt.Errorf("prune: %w", err)
	}

	var errs []error
	mainPeers, err := n.service.ListMainPeers(name)
	if err != nil {
		errs = append(errs, fmt.Errorf("list main peers: %w", err))
	} else if err := n.main.device.SetPeers(mainPeers...); err != nil {
		errs = append(errs, fmt.Errorf("apply main peers: %w", err))
	} else if err := n.observeLocked(now); err != nil {
		errs = append(errs, err)
	}

	invitePeers, expiry, err := n.service.ListRegistrationPeers(name)
	if err != nil {
		errs = append(errs, fmt.Errorf("list invite peers: %w", err))
	} else if err := n.invite.device.SetPeers(invitePeers...); err != nil {
		errs = append(errs, fmt.Errorf("apply invite peers: %w", err))
	}

	n.log.Debug(
		"reconcile",
		"peers", len(mainPeers),
		"registrations", len(invitePeers),
	)

	return expiry, errors.Join(errs...)
}

// observeLocked reports the endpoints the main device currently sees for its
// peers. Only handshakes fresh enough to still be routable count; the
// service decides which of the observed keys are peers worth recording.
func (n *Network) observeLocked(
	now time.Time,
) error {
	livePeers, err := n.main.device.GetPeerStatuses()
	if err != nil {
		return fmt.Errorf("read main peers: %w", err)
	}

	sightings := make([]service.EndpointSighting, 0, len(livePeers))
	for _, peer := range livePeers {
		if peer.Endpoint == nil || !peerHealthy(peer, now) {
			continue
		}
		sightings = append(sightings, service.EndpointSighting{
			WitnessKey: n.record.PublicKey,
			PeerKey:    peer.PublicKey.String(),
			Endpoint:   peer.Endpoint.String(),
			Timestamp:  now,
		})
	}

	if len(sightings) == 0 {
		return nil
	}

	if err := n.service.ObserveEndpoints(
		n.record.Name,
		sightings,
	); err != nil {
		return fmt.Errorf("store main peer endpoints: %w", err)
	}

	return nil
}

// rearmLocked requires mu and schedules expiry or the maximum interval.
func (n *Network) rearmLocked(
	now time.Time,
	expiry time.Time,
) {
	// A stopped or canceled lifetime must not schedule more work.
	if n.ctx == nil || n.ctx.Err() != nil {
		return
	}

	next := now.Add(NetworkReconcileInterval)
	if !expiry.IsZero() && expiry.Before(next) {
		next = expiry
	}

	delay := next.Sub(now)
	if delay <= 0 {
		delay = time.Second
	}

	if n.timer == nil {
		// Capture this lifetime so the callback cannot act on a later one.
		ctx := n.ctx
		n.timer = time.AfterFunc(delay, func() {
			n.reconcileScheduled(ctx)
		})
		return
	}

	n.timer.Reset(delay)
}

// reconcileScheduled only maintains the lifetime that scheduled it. Unlike an
// explicit Reconcile, a timer callback cannot start or restart a network.
func (n *Network) reconcileScheduled(
	ctx context.Context,
) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// A queued callback may acquire the lock after Stop or a restart.
	// Reject callbacks that are not for the current lifetime.
	if n.ctx != ctx || ctx.Err() != nil {
		return
	}

	if err := n.reconcileLocked(); err != nil {
		n.log.Warn("scheduled reconciliation failed", "err", err)
	}
}

// peerHealthy reports whether a peer's last handshake is recent enough
// to consider it connected. Peers that have never handshaken are not
// healthy.
func peerHealthy(
	peerStatus wireguard.PeerStatus,
	now time.Time,
) bool {
	return !peerStatus.LastHandshake.IsZero() &&
		now.Sub(peerStatus.LastHandshake) < wireguard.ActiveHandshakeThreshold
}

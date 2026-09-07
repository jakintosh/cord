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

// reconcileCap is the maximum duration between reconciliation passes for
// a running network. The self-rearming timer uses min(earliest
// registration expiry, now+cap).
const reconcileCap = 5 * time.Minute

// Network is a server network running in this process: two Planes (main
// and invite), plus a self-rearming reconciliation timer. It holds no durable state of its own — every
// read and write goes through the service.
type Network struct {
	record  *service.Network
	main    *Plane
	invite  *Plane
	service *service.Service
	clock   func() time.Time
	log     *slog.Logger

	dnsPort     uint16
	dnsDisabled bool

	cancel context.CancelFunc

	// mu is held for the whole of reconcile, so stop() waits for a pass
	// in flight and no pass survives the stop.
	mu              sync.Mutex
	timer           *time.Timer
	stopped         bool
	reconcileStatus ActivityStatus
	mainAPIStatus   ActivityStatus
	inviteAPIStatus ActivityStatus
}

// start brings up both planes and their servers under ctx, then runs
// the first reconciliation. A failure at any step rolls the whole boot
// back by stopping both planes, including any partially started plane.
func (n *Network) start(
	ctx context.Context,
	wg *wireguard.Manager,
	mainHandler http.Handler,
	inviteHandler http.Handler,
) error {
	ctx, n.cancel = context.WithCancel(ctx)

	// Every component's data is initialized before any of them starts,
	// so rollback can stop both planes unconditionally.
	n.main = &Plane{
		config:     n.record.Main,
		clock:      n.clock,
		privateKey: n.record.PrivateKey,
		log:        n.log.With("plane", "main"),
		onServeResult: func(err error) {
			n.recordActivity(&n.mainAPIStatus, err)
		},
	}

	if !n.dnsDisabled {
		n.main.dnsConfig = &dnsConfig{
			network: n.record.Name,
			service: n.service,
			port:    n.dnsPort,
		}
	}

	n.invite = &Plane{
		config:     n.record.Invite,
		clock:      n.clock,
		privateKey: n.record.PrivateKey,
		log:        n.log.With("plane", "invite"),
		onServeResult: func(err error) {
			n.recordActivity(&n.inviteAPIStatus, err)
		},
	}

	// rollback undoes a partially successful boot: it cancels the
	// network context and stops every component in the same order as
	// stop(). Every stop is safe when its component never started.
	rollback := func(cause error) error {
		n.cancel()
		return errors.Join(cause, n.stopComponents())
	}

	if err := n.main.start(ctx, wg, mainHandler); err != nil {
		return rollback(fmt.Errorf("main plane: %w", err))
	}

	if err := n.invite.start(ctx, wg, inviteHandler); err != nil {
		return rollback(fmt.Errorf("invite plane: %w", err))
	}

	n.reconcile()

	return nil
}

// stop cancels the network context, retires the timer, waits for any
// reconciliation in flight, and stops both planes and their servers.
// Errors are joined.
func (n *Network) stop() error {
	n.mu.Lock()
	n.stopped = true
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
	n.mu.Unlock()

	n.cancel()

	return n.stopComponents()
}

// stopComponents releases both planes, including their dependent servers.
func (n *Network) stopComponents() error {
	return errors.Join(n.main.stop(), n.invite.stop())
}

// reconcile is the ONLY code path that writes peer state to devices. It
// prunes aged-out state, applies the desired peer set to both planes,
// records the endpoints the main device has seen, and rearms the timer.
// A stopped network reconciles no further, so the timer cannot outlive
// the devices it configures.
func (n *Network) reconcile() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.stopped {
		return
	}

	name := n.record.Name
	now := n.clock()
	var expiry time.Time

	// refresh records the pass outcome in the status and rearms the
	// timer. The first failure of a pass wins the status slot; later
	// failures are logged but don't displace it.
	var passErr error
	refresh := func(err error) {
		if passErr == nil {
			passErr = err
		}
		n.reconcileStatus.record(n.clock(), passErr)
		n.rearm(now, expiry)
	}

	if err := n.service.PruneNetwork(name); err != nil {
		n.log.Warn("reconcile: prune failed", "err", err)
		refresh(fmt.Errorf("prune: %w", err))
		return
	}

	mainPeers, err := n.service.ListMainPeers(name)
	if err != nil {
		n.log.Warn("reconcile: main peers failed", "err", err)
		refresh(fmt.Errorf("list main peers: %w", err))
		return
	}

	if err := n.main.device.SetPeers(mainPeers...); err != nil {
		n.log.Warn("reconcile: apply main peers failed", "err", err)
		refresh(fmt.Errorf("apply main peers: %w", err))
	} else if err := n.observe(now); err != nil {
		n.log.Warn(
			"reconcile: observe failed",
			"err",
			err,
		)
		refresh(err)
	}

	invitePeers, expiry, err := n.service.ListRegistrationPeers(name)
	if err != nil {
		n.log.Warn("reconcile: invite peers failed", "err", err)
		refresh(fmt.Errorf("list invite peers: %w", err))
		return
	}

	if err := n.invite.device.SetPeers(invitePeers...); err != nil {
		n.log.Warn("reconcile: apply invite peers failed", "err", err)
		refresh(fmt.Errorf("apply invite peers: %w", err))
	}

	mainErr := n.main.check()
	if mainErr != nil {
		mainErr = fmt.Errorf("main plane: %w", mainErr)
	}

	inviteErr := n.invite.check()
	if inviteErr != nil {
		inviteErr = fmt.Errorf("invite plane: %w", inviteErr)
	}

	if err := errors.Join(mainErr, inviteErr); err != nil {
		n.log.Warn("reconcile: plane check failed", "err", err)
		refresh(err)
	}

	n.log.Debug("reconcile", "peers", len(mainPeers), "registrations", len(invitePeers))

	refresh(nil)
}

// observe reports the endpoints the main device currently sees for its
// peers. Only handshakes fresh enough to still be routable count; the
// service decides which of the observed keys are peers worth recording.
func (n *Network) observe(
	now time.Time,
) error {
	livePeers, err := n.main.device.Peers()
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

// rearm schedules the next reconciliation: at the earliest registration
// expiry, or the cap, whichever comes first. Callers hold n.mu.
func (n *Network) rearm(
	now time.Time,
	expiry time.Time,
) {
	next := now.Add(reconcileCap)
	if !expiry.IsZero() && expiry.Before(next) {
		next = expiry
	}

	delay := next.Sub(now)
	if delay <= 0 {
		delay = time.Second
	}

	if n.timer == nil {
		n.timer = time.AfterFunc(delay, n.reconcile)
		return
	}
	n.timer.Reset(delay)
}

// getActivityStatuses returns a consistent snapshot of this network's work.
func (n *Network) getActivityStatuses() (
	ActivityStatus,
	ActivityStatus,
	ActivityStatus,
	ActivityStatus,
) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.reconcileStatus, n.mainAPIStatus, n.inviteAPIStatus, n.main.dnsStatus
}

func (n *Network) recordActivity(
	status *ActivityStatus,
	err error,
) {
	n.mu.Lock()
	defer n.mu.Unlock()
	status.record(n.clock(), err)
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

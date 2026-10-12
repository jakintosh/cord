package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/dns"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

type networkOptions struct {
	record          *service.Network
	service         *service.Service
	wireguard       *wireguard.Manager
	mainHandler     http.Handler
	inviteHandler   http.Handler
	dnsPort         uint16
	dnsDisabled     bool
	shutdownTimeout time.Duration
	interval        time.Duration
	clock           func() time.Time
	log             *slog.Logger
}

// Network holds two planes. Only the runtime loop operates on it.
type Network struct {
	record  *service.Network
	main    *Plane
	invite  *Plane
	service *service.Service
	clock   func() time.Time
	log     *slog.Logger

	closing         bool // Retained only when cleanup needs another attempt.
	reconcileStatus resource.ActivityStatus
	cleanupStatus   resource.ActivityStatus
}

// newNetwork constructs both planes without starting resources or scheduling work.
func newNetwork(
	opts networkOptions,
) (
	*Network,
	error,
) {
	var dnsOptions *dns.Options
	if !opts.dnsDisabled {
		route, err := netaddr.InterfaceRouteFromCidr(opts.record.Main.Cidr)
		if err != nil {
			return nil, fmt.Errorf("main plane: plane address: %w", err)
		}
		dnsOptions = &dns.Options{
			Network: opts.record.Name,
			Zone:    opts.record.Name + ".internal",
			Address: netaddr.Endpoint(route.IP, opts.dnsPort),
			Service: opts.service,
			Logger:  opts.log.With("plane", "main"),
		}
	}

	main, err := newPlane(
		opts.record.Main,
		dnsOptions,
		opts.record.PrivateKey,
		opts.mainHandler,
		opts.wireguard,
		opts.shutdownTimeout,
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
		opts.shutdownTimeout,
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
		reconcileStatus: resource.ActivityStatus{
			Interval: opts.interval,
		},
	}, nil
}

// status collects plane snapshots and activity without changing resources.
func (n *Network) status() resource.Snapshot {
	target := resource.Running
	if n.closing {
		target = resource.Stopped
	}

	status := resource.Snapshot{
		Kind:   "network",
		Name:   n.record.Name,
		State:  resource.Running,
		Target: target,
		Activities: map[string]resource.ActivityStatus{
			"reconcile": n.reconcileStatus,
			"close":     n.cleanupStatus,
		},
		Children: []resource.Snapshot{
			n.main.status("main", target),
			n.invite.status("invite", target),
		},
	}

	if n.closing {
		status.State = resource.Stopped
	}

	if n.cleanupStatus.Error != "" {
		status.State = resource.Failed
	}

	status.UpdateHealth()

	return status
}

// reconcile repairs infrastructure before applying saved peer configuration.
// It is called only by runtime maintenance and administrative commands.
func (n *Network) reconcile(
	ctx context.Context,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	err := errors.Join(
		n.main.reconcile(ctx),
		n.invite.reconcile(ctx),
		n.applyPeers(),
	)

	n.reconcileStatus.Record(n.clock(), err)

	return err
}

// close releases servers before devices. Failed cleanup is retried by runtime.
func (n *Network) close() (err error) {
	defer func() {
		n.cleanupStatus.Record(n.clock(), err)
	}()

	n.closing = true
	return errors.Join(
		n.main.close(),
		n.invite.close(),
	)
}

// applyPeers never changes the serving infrastructure. Pruning must succeed before
// applying peer state. After that, a failure on one plane does not prevent updating
// the other.
func (n *Network) applyPeers() error {
	now := n.clock()
	name := n.record.Name

	if err := n.service.PruneNetwork(name); err != nil {
		return fmt.Errorf("prune: %w", err)
	}

	var errs []error
	mainPeers, err := n.service.ListMainPeers(name)
	if err != nil {
		errs = append(errs, fmt.Errorf("list main peers: %w", err))
	} else if err := n.main.device.SetPeers(mainPeers...); err != nil {
		errs = append(errs, fmt.Errorf("apply main peers: %w", err))
	} else if err := n.observe(now); err != nil {
		errs = append(errs, err)
	}

	invitePeers, err := n.service.ListRegistrationPeers(name)
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

	return errors.Join(errs...)
}

// observe reports the endpoints the main device currently sees for its
// peers. Only handshakes fresh enough to still be routable count; the
// service decides which of the observed keys are peers worth recording.
func (n *Network) observe(
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

// SetNetworkEnabled saves intent and returns the resulting network status.
func (r *Runtime) SetNetworkEnabled(
	ctx context.Context,
	name string,
	enabled bool,
) (
	NetworkStatus,
	error,
) {
	return executeValue(r, ctx, func() (NetworkStatus, error) {

		if err := r.service.SetNetworkEnabled(
			name,
			enabled,
		); err != nil {
			return NetworkStatus{}, err
		}

		record, err := r.service.GetNetwork(name)
		if err != nil {
			return NetworkStatus{}, err
		}

		if err := r.reconcileNetwork(record); err != nil {
			r.reasons[name] = fmt.Sprintf("apply failed: %v", err)
			r.log.Error(
				"apply network intent failed",
				"network", name,
				"err", err,
			)
		}

		var snapshot *resource.Snapshot
		if network := r.running[name]; network != nil {
			status := network.status()
			snapshot = &status
		}
		return r.networkStatus(record, snapshot), nil
	})
}

// GetNetwork returns the persisted network record by name.
func (r *Runtime) GetNetwork(
	ctx context.Context,
	name string,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (*service.Network, error) {
		return r.service.GetNetwork(name)
	})
}

// ListNetworks returns persisted network intent.
func (r *Runtime) ListNetworks(
	ctx context.Context,
) (
	[]*service.Network,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Network, error) {
		return r.service.ListNetworks()
	})
}

// CreateNetwork defines a new server network: validates the config,
// generates the server keypair, and persists the record. Zero-valued
// fields in the Planes mean "use default."
func (r *Runtime) CreateNetwork(
	ctx context.Context,
	name string,
	externalIP string,
	main service.Plane,
	invite service.Plane,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (*service.Network, error) {
		return r.service.CreateNetwork(name, externalIP, main, invite)
	})
}

// DeleteNetwork removes the named network and all of its resources.
// The network must be disabled first; the store enforces the guard
// atomically with the delete.
func (r *Runtime) DeleteNetwork(
	ctx context.Context,
	name string,
) error {
	return execute(r, ctx, func() error {
		return r.service.DeleteNetwork(name)
	})
}

// reconcileNetwork applies one record under the active runtime lifetime.
func (r *Runtime) reconcileNetwork(
	record *service.Network,
) error {
	network, isManaged := r.running[record.Name]

	switch {

	// Disabled but still managed; bring it down.
	case !record.Enabled && isManaged:
		delete(r.reasons, record.Name)
		return r.closeNetwork(record.Name, network)

	// Disabled and already down; forget any stale reason.
	case !record.Enabled && !isManaged:
		delete(r.reasons, record.Name)
		break // nothing to do

	// Enabled and managed; reconcile it.
	case record.Enabled && isManaged:
		if network.closing {
			// Retry unfinished cleanup before attempting another start.
			if err := r.closeNetwork(record.Name, network); err != nil {
				return err
			}
			return r.startNetwork(record)
		}

		if err := network.reconcile(r.ctx); err != nil {
			return err
		}
		//
		// reconcile succeeded
		delete(r.reasons, record.Name)
		break

	// Enabled and down; bring it up.
	case record.Enabled && !isManaged:
		return r.startNetwork(record)
	}

	return nil
}

// startNetwork brings a network up under its record and into the
// running set, or records the failure as its reason to be retried on
// the next pass.
func (r *Runtime) startNetwork(
	record *service.Network,
) error {
	var mainHandler, inviteHandler http.Handler
	if r.handlers.Peer != nil {
		mainHandler = r.handlers.Peer(record.Name)
	}

	if r.handlers.Invite != nil {
		inviteHandler = r.handlers.Invite(record.Name)
	}

	network, err := newNetwork(networkOptions{
		record:          record,
		service:         r.service,
		wireguard:       r.wireguard,
		mainHandler:     mainHandler,
		inviteHandler:   inviteHandler,
		dnsPort:         r.dnsPort,
		dnsDisabled:     r.dnsDisabled,
		shutdownTimeout: r.shutdownTimeout,
		interval:        r.interval,
		clock:           r.clock,
		log:             r.log.With("network", record.Name),
	})
	if err != nil {
		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	if err := network.reconcile(r.ctx); err != nil {
		if closeErr := network.close(); closeErr != nil {
			r.running[record.Name] = network
			err = errors.Join(err, closeErr)
		}

		r.reasons[record.Name] = fmt.Sprintf("start failed: %v", err)
		return err
	}

	r.running[record.Name] = network
	delete(r.reasons, record.Name)

	r.log.Info(
		"network started",
		"network", record.Name,
	)

	return nil
}

// closeNetwork takes a running network down and forgets it after cleanup.
func (r *Runtime) closeNetwork(
	name string,
	network *Network,
) error {
	err := network.close()
	if err != nil {
		return fmt.Errorf("stop network %q: %w", name, err)
	}

	delete(r.running, name)
	delete(r.reasons, name)

	r.log.Info(
		"network stopped",
		"network", name,
	)

	return nil
}

// applyPeers is called by plane handlers. Never stop or replace a plane here:
// HTTP draining would wait for the handler that is waiting on this command.
// Saved command results remain valid when application fails.
func (r *Runtime) applyPeers(
	name string,
) {
	n := r.running[name]
	if n == nil || n.closing {
		return
	}

	err := n.applyPeers()
	n.reconcileStatus.Record(r.clock(), err)
	if err != nil {
		r.reasons[name] = fmt.Sprintf("apply failed: %v", err)
		r.log.Error(
			"apply network peers failed",
			"network", name,
			"err", err,
		)
	} else {
		delete(r.reasons, name)
	}
}

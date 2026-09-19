package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Enrollment owns the temporary resources used to turn a saved installation
// into a permanent network. The runtime serializes its methods and retains it
// until cleanup succeeds.
type Enrollment struct {
	install    *service.Install
	tunnel     *Tunnel
	service    *service.Service
	wireguard  *wireguard.Manager
	httpClient *http.Client
	clock      func() time.Time

	redeemStatus  ActivityStatus
	confirmStatus ActivityStatus
	cleanupStatus ActivityStatus
}

// redeem redeems the install's invite over a one-shot tunnel and
// persists the main-network assignment the server returns. The tunnel
// comes down before redeem succeeds. Idempotent: an
// already-redeemed install is returned unchanged without contacting the
// server again.
func (e *Enrollment) redeem(
	ctx context.Context,
) (
	result *service.Install,
	err error,
) {
	defer func() {
		e.redeemStatus.record(e.clock(), err)
	}()

	if err := e.close(); err != nil {
		return nil, err
	}

	defer func() {
		err = errors.Join(err, e.close())
	}()

	switch e.install.Phase {
	case service.PhaseInvited:
		break // ok, continue

	case service.PhaseRedeemed:
		return e.install, nil // idempotent redemption

	default:
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q",
			service.ErrInstallState,
			e.install.Name,
			e.install.Phase,
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// create new public key
	permPubKey, err := wireguard.PublicKey(e.install.MainPrivateKey)
	if err != nil {
		return nil, err
	}

	// determine host route for invite tunnel
	inviteHostRoute, err := netaddr.HostRouteFromCidr(e.install.InviteAssignedRoute)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid invite route %q",
			service.ErrInvalidInput,
			e.install.InviteAssignedRoute,
		)
	}

	// create invite tunnel
	tunnel, err := newTunnel(tunnelOptions{
		name:       e.install.InviteIfaceName,
		privateKey: e.install.InvitePrivateKey,
		route:      inviteHostRoute.String(),
		listenPort: e.install.ListenPort,
		server:     e.install.InviteServer,
	}, e.wireguard)
	if err != nil {
		return nil, fmt.Errorf("create invite tunnel: %w", err)
	}

	e.tunnel = tunnel
	if err := tunnel.reconcile(ctx); err != nil {
		return nil, err
	}

	if err := tunnel.device.SetPeers(wireguard.PeerConfig{
		PublicKey:      e.install.InviteServer.PublicKey,
		AllowedIPs:     []string{e.install.InviteServer.Route},
		Endpoint:       e.install.InviteServer.Endpoint,
		EndpointPolicy: wireguard.EndpointFixed,
	}); err != nil {
		return nil, fmt.Errorf("set invite server peer: %w", err)
	}

	inviteNetClient, err := client.NewInviteClient(tunnel.apiAddr, e.httpClient)
	if err != nil {
		return nil, fmt.Errorf("create invite client: %w", err)
	}

	invitation, err := inviteNetClient.RedeemInvitation(ctx, permPubKey)
	if err != nil {
		return nil, fmt.Errorf("redeem invite: %w", err)
	}

	redeemed, err := e.service.RedeemInstall(e.install.Name, *invitation)
	if err != nil {
		return nil, err
	}
	e.install = redeemed
	return redeemed, nil
}

// confirm proves reachability on the main network over a one-shot
// tunnel and consumes the install into a permanent membership. Only a
// redeemed install can be confirmed.
func (e *Enrollment) confirm(
	ctx context.Context,
) (
	result *service.Network,
	err error,
) {
	defer func() {
		e.confirmStatus.record(e.clock(), err)
	}()

	if err := e.close(); err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, e.close())
	}()

	switch e.install.Phase {
	case service.PhaseRedeemed:
		break // ok, continue

	default:
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q, expected redeemed",
			service.ErrInstallState,
			e.install.Name,
			e.install.Phase,
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tunnel, err := newTunnel(tunnelOptions{
		name:       e.install.MainIfaceName,
		privateKey: e.install.MainPrivateKey,
		route:      e.install.MainAssignedRoute,
		listenPort: e.install.ListenPort,
		server:     e.install.MainServer,
	}, e.wireguard)
	if err != nil {
		return nil, fmt.Errorf("create main tunnel: %w", err)
	}

	e.tunnel = tunnel
	if err := tunnel.reconcile(ctx); err != nil {
		return nil, err
	}

	if err := tunnel.device.SetPeers(wireguard.PeerConfig{
		PublicKey:           e.install.MainServer.PublicKey,
		AllowedIPs:          []string{e.install.MainServer.Route},
		Endpoint:            e.install.MainServer.Endpoint,
		EndpointPolicy:      wireguard.EndpointFixed,
		PersistentKeepalive: int(PersistentKeepaliveInterval / time.Second),
	}); err != nil {
		return nil, fmt.Errorf("set main server peer: %w", err)
	}

	peerNetClient, err := client.NewPeerClient(tunnel.apiAddr, e.httpClient)
	if err != nil {
		return nil, fmt.Errorf("create peer client: %w", err)
	}

	if err := peerNetClient.ConfirmPeer(ctx); err != nil {
		return nil, fmt.Errorf("confirm peer: %w", err)
	}

	// Release the temporary interface before publishing enabled intent and
	// reconciling the permanent network.
	if err := e.close(); err != nil {
		return nil, fmt.Errorf("close main tunnel: %w", err)
	}

	return e.service.ConfirmInstall(e.install.Name)
}

// close retains the child when cleanup fails.
func (e *Enrollment) close() (
	err error,
) {
	if e.tunnel == nil {
		return nil
	}
	defer func() {
		e.cleanupStatus.record(e.clock(), err)
	}()

	if err := e.tunnel.close(); err != nil {
		return err
	}
	e.tunnel = nil
	return nil
}

// usesInterface describes the interfaces reserved for either enrollment step.
func (e *Enrollment) usesInterface(name string) bool {
	return name != "" && (name == e.install.InviteIfaceName || name == e.install.MainIfaceName)
}

// Install runs the full onboarding flow: BeginInstall → Redeem →
// Confirm, threading each step's result to the next instead of
// re-reading from the store. It is a convenience driver for callers
// that want to complete onboarding in a single call. For retry-safe
// onboarding, call each step individually — the permanent key is
// persisted at BeginInstall and reused across retries.
//
// If the install already exists (from a previous partial run), Install
// resumes from whatever phase it is at.
func (r *Runtime) Install(
	ctx context.Context,
	invitation protocol.Invitation,
	options service.NetworkOptions,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (result *service.Network, err error) {
		// Stop outbound requests on caller or runtime cancellation.
		requestCtx, cancel := context.WithCancel(ctx)
		removeShutdownHook := context.AfterFunc(r.ctx, cancel)
		defer removeShutdownHook()
		defer cancel()

		install, err := r.service.BeginInstall(invitation, options)
		if err != nil {
			return nil, err
		}

		enrollment, err := r.acquireEnrollment(install)
		if err != nil {
			return nil, err
		}
		defer func() {
			err = errors.Join(err, r.releaseEnrollment(install.Name))
		}()

		_, err = enrollment.redeem(requestCtx)
		if err != nil {
			return nil, err
		}

		network, err := enrollment.confirm(requestCtx)
		if err != nil {
			return nil, err
		}

		if err := r.releaseEnrollment(install.Name); err != nil {
			return nil, err
		}

		if err := r.convergeName(network.Name); err != nil {
			return nil, err
		}

		return network, nil
	})
}

// RedeemInstall brings up the temporary invite tunnel, calls /redeem with the
// stored permanent public key, and hands the result to the service to
// persist. Successful return includes tunnel cleanup; failures remain owned.
// Idempotent: an
// already-redeemed install is returned unchanged without contacting the
// server again.
func (r *Runtime) RedeemInstall(
	ctx context.Context,
	name string,
) (
	*service.Install,
	error,
) {
	return executeValue(r, ctx, func() (result *service.Install, err error) {
		// Stop outbound requests on caller or runtime cancellation.
		requestCtx, cancel := context.WithCancel(ctx)
		removeShutdownHook := context.AfterFunc(r.ctx, cancel)
		defer removeShutdownHook()
		defer cancel()

		install, err := r.service.GetInstall(name)
		if err != nil {
			return nil, err
		}

		enrollment, err := r.acquireEnrollment(install)
		if err != nil {
			return nil, err
		}

		defer func() {
			err = errors.Join(err, r.releaseEnrollment(install.Name))
		}()

		return enrollment.redeem(requestCtx)
	})
}

// ConfirmInstall brings up the main tunnel, calls /confirm to prove
// reachability, and has the service consume the install into a permanent
// membership, returning the resulting network. The one-shot tunnel comes
// down before the network is converged, so the running network is the
// one the runtime owns.
//
// If the confirm call or store transaction fails, the install remains redeemed.
// Failed tunnel cleanup remains owned for retry.
func (r *Runtime) ConfirmInstall(
	ctx context.Context,
	name string,
) (
	*service.Network,
	error,
) {
	return executeValue(r, ctx, func() (result *service.Network, err error) {
		// Stop outbound requests on caller or runtime cancellation.
		requestCtx, cancel := context.WithCancel(ctx)
		removeShutdownHook := context.AfterFunc(r.ctx, cancel)
		defer removeShutdownHook()
		defer cancel()

		install, err := r.service.GetInstall(name)
		if err != nil {
			return nil, err
		}

		enrollment, err := r.acquireEnrollment(install)
		if err != nil {
			return nil, err
		}
		defer func() {
			err = errors.Join(err, r.releaseEnrollment(install.Name))
		}()

		network, err := enrollment.confirm(requestCtx)
		if err != nil {
			return nil, err
		}

		if err := r.releaseEnrollment(install.Name); err != nil {
			return nil, err
		}

		if err := r.convergeName(network.Name); err != nil {
			return nil, err
		}

		return network, nil
	})
}

// acquireEnrollment finishes retained cleanup before reserving the interfaces
// for a fresh execution of the saved installation.
func (r *Runtime) acquireEnrollment(
	install *service.Install,
) (
	*Enrollment,
	error,
) {
	if err := r.releaseEnrollment(install.Name); err != nil {
		return nil, err
	}

	if err := r.prepareEnrollmentInterfaces(install); err != nil {
		return nil, err
	}

	enrollment := &Enrollment{
		install:    install,
		service:    r.service,
		wireguard:  r.wireguard,
		httpClient: r.httpClient,
		clock:      r.clock,
	}
	r.enrollments[install.Name] = enrollment
	return enrollment, nil
}

// prepareEnrollmentInterfaces rejects network claims before releasing any
// conflicting enrollments. Failed cleanup continues to reserve its interfaces.
func (r *Runtime) prepareEnrollmentInterfaces(
	install *service.Install,
) error {
	interfaces := []string{
		install.InviteIfaceName,
		install.MainIfaceName,
	}

	for _, iface := range interfaces {
		if iface == "" {
			continue
		}

		for _, network := range r.running {
			if network.record.InterfaceName == iface {
				return fmt.Errorf(
					"%w: interface %q is owned by a network",
					service.ErrConflict,
					iface,
				)
			}
		}
	}

	for _, iface := range interfaces {
		if err := r.releaseEnrollmentsUsing(iface); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) releaseEnrollment(
	name string,
) error {
	enrollment, ok := r.enrollments[name]
	if !ok {
		return nil
	}

	if err := enrollment.close(); err != nil {
		return fmt.Errorf("close enrollment %q: %w", name, err)
	}

	delete(r.enrollments, name)
	return nil
}

// releaseEnrollmentsUsing prevents interface reuse before cleanup succeeds.
func (r *Runtime) releaseEnrollmentsUsing(
	iface string,
) error {
	for name, enrollment := range r.enrollments {
		if enrollment.usesInterface(iface) {
			if err := r.releaseEnrollment(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// closeEnrollments attempts every retained child even when one close fails.
func (r *Runtime) closeEnrollments() (
	err error,
) {
	defer func() {
		r.cleanupStatus.record(r.clock(), err)
	}()

	var errs []error
	for name := range r.enrollments {
		if err := r.releaseEnrollment(name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

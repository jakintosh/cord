package runtime

import (
	"context"
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

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
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	install, err := r.service.BeginInstall(invitation, options)
	if err != nil {
		return nil, err
	}

	install, err = r.redeemInstall(ctx, install)
	if err != nil {
		return nil, err
	}

	network, err := r.confirmInstall(ctx, install)
	if err != nil {
		return nil, err
	}

	return network, nil
}

// RedeemInstall brings up the temporary invite tunnel, calls /redeem with the
// stored permanent public key, and hands the result to the service to
// persist. The tunnel comes down before RedeemInstall returns. Idempotent: an
// already-redeemed install is returned unchanged without contacting the
// server again.
func (r *Runtime) RedeemInstall(
	ctx context.Context,
	name string,
) (
	*service.Install,
	error,
) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	install, err := r.service.GetInstall(name)
	if err != nil {
		return nil, err
	}

	return r.redeemInstall(ctx, install)
}

// ConfirmInstall brings up the main tunnel, calls /confirm to prove
// reachability, and has the service consume the install into a permanent
// membership, returning the resulting network. The one-shot tunnel comes
// down before the network is converged, so the running network is the
// one the runtime owns.
//
// If the confirm call or store transaction fails, the tunnel comes down
// and the install row remains at "redeemed" — retryable.
func (r *Runtime) ConfirmInstall(
	ctx context.Context,
	name string,
) (
	*service.Network,
	error,
) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	install, err := r.service.GetInstall(name)
	if err != nil {
		return nil, err
	}

	return r.confirmInstall(ctx, install)
}

// redeemInstall redeems the install's invite over a one-shot tunnel and
// persists the main-network assignment the server returns. The tunnel
// comes down before redeemInstall returns. Idempotent: an
// already-redeemed install is returned unchanged without contacting the
// server again.
func (r *Runtime) redeemInstall(
	ctx context.Context,
	install *service.Install,
) (
	*service.Install,
	error,
) {
	switch install.Phase {
	case service.PhaseInvited:
		break // ok, continue

	case service.PhaseRedeemed:
		return install, nil // idempotent redemption

	default:
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q",
			service.ErrInstallState,
			install.Name,
			install.Phase,
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// create new public key
	permPubKey, err := wireguard.PublicKey(install.MainPrivateKey)
	if err != nil {
		return nil, err
	}

	// determine host route for invite tunnel
	inviteHostRoute, err := netaddr.HostRouteFromCidr(install.InviteAssignedRoute)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: invalid invite route %q",
			service.ErrInvalidInput,
			install.InviteAssignedRoute,
		)
	}

	// create invite tunnel
	tunnel := &Tunnel{
		name:       install.InviteIfaceName,
		privateKey: install.InvitePrivateKey,
		route:      inviteHostRoute.String(),
		listenPort: install.ListenPort,
		server:     install.InviteServer,
	}
	if err := tunnel.start(r.wireguard); err != nil {
		return nil, fmt.Errorf("create invite tunnel: %w", err)
	}
	defer func() {
		if err := tunnel.stop(); err != nil {
			r.log.Warn(
				"close invite tunnel",
				"device",
				install.InviteIfaceName,
				"error",
				err,
			)
		}
	}()

	inviteNetClient, err := client.NewInviteClient(tunnel.apiAddr, r.httpClient)
	if err != nil {
		return nil, fmt.Errorf("create invite client: %w", err)
	}

	invitation, err := inviteNetClient.RedeemInvitation(ctx, permPubKey)
	if err != nil {
		return nil, fmt.Errorf("redeem invite: %w", err)
	}

	return r.service.RedeemInstall(install.Name, *invitation)
}

// confirmInstall proves reachability on the main network over a one-shot
// tunnel and consumes the install into a permanent membership. Only a
// redeemed install can be confirmed.
func (r *Runtime) confirmInstall(
	ctx context.Context,
	install *service.Install,
) (
	*service.Network,
	error,
) {
	switch install.Phase {
	case service.PhaseRedeemed:
		break // ok, continue

	default:
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q, expected redeemed",
			service.ErrInstallState,
			install.Name,
			install.Phase,
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tunnel := &Tunnel{
		name:       install.MainIfaceName,
		privateKey: install.MainPrivateKey,
		route:      install.MainAssignedRoute,
		listenPort: install.ListenPort,
		server:     install.MainServer,
		keepalive:  PersistentKeepaliveInterval,
	}
	if err := tunnel.start(r.wireguard); err != nil {
		return nil, fmt.Errorf("create main tunnel: %w", err)
	}
	defer func() {
		if err := tunnel.stop(); err != nil {
			r.log.Warn(
				"close main tunnel",
				"device",
				install.MainIfaceName,
				"error",
				err,
			)
		}
	}()

	peerNetClient, err := client.NewPeerClient(tunnel.apiAddr, r.httpClient)
	if err != nil {
		return nil, fmt.Errorf("create peer client: %w", err)
	}

	if err := peerNetClient.ConfirmPeer(ctx); err != nil {
		return nil, fmt.Errorf("confirm peer: %w", err)
	}

	network, err := r.service.ConfirmInstall(install.Name)
	if err != nil {
		return nil, err
	}

	if err := r.Converge(install.Name); err != nil {
		return nil, err
	}

	return network, nil
}

package runtime

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// ListRegistrationGroups returns the groups held by a registration before
// confirmation. Confirmed registrations have no registration-held groups.
func (r *Runtime) ListRegistrationGroups(
	ctx context.Context,
	networkName string,
	regName string,
) (
	[]*service.Group,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Group, error) {
		return r.service.ListRegistrationGroups(networkName, regName)
	})
}

// AssignRegistrationGroup records a group assignment as registration intent.
// The assignment is transferred to the peer's terminal CIDR at confirmation.
func (r *Runtime) AssignRegistrationGroup(
	ctx context.Context,
	networkName string,
	regName string,
	group string,
) error {
	return execute(r, ctx, func() error {
		return r.service.AssignRegistrationGroup(networkName, regName, group)
	})
}

// RemoveRegistrationGroup removes group intent from an unconfirmed
// registration. Removing an assignment that is not present is idempotent.
func (r *Runtime) RemoveRegistrationGroup(
	ctx context.Context,
	networkName string,
	regName string,
	group string,
) error {
	return execute(r, ctx, func() error {
		return r.service.RemoveRegistrationGroup(networkName, regName, group)
	})
}

// ListRegistrations returns all registrations for the given network
// (active, expired, and redeemed).
func (r *Runtime) ListRegistrations(
	ctx context.Context,
	networkName string,
) (
	[]*service.Registration,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Registration, error) {
		return r.service.ListRegistrations(networkName)
	})
}

// CreateRegistration saves the domain change and applies local peer configuration.
// Application failures are recorded without discarding the saved result.
func (r *Runtime) CreateRegistration(
	ctx context.Context,
	networkName string,
	regName string,
	opts service.RegistrationOptions,
) (
	*protocol.Invitation,
	error,
) {
	return executeValue(r, ctx, func() (*protocol.Invitation, error) {
		result, err := r.service.CreateRegistration(networkName, regName, opts)
		if err != nil {
			return nil, err
		}

		r.applyPeers(networkName)
		return result, nil
	})
}

// RedeemRegistration saves the domain change and applies local peer configuration.
// Application failures are recorded without discarding the saved result.
func (r *Runtime) RedeemRegistration(
	ctx context.Context,
	networkName string,
	tempPubKey string,
	permPubKey string,
) (
	*protocol.Invitation,
	error,
) {
	return executeValue(r, ctx, func() (*protocol.Invitation, error) {
		result, err := r.service.RedeemRegistration(networkName, tempPubKey, permPubKey)
		if err != nil {
			return nil, err
		}

		r.applyPeers(networkName)
		return result, nil
	})
}

// RevokeRegistration saves the domain change and applies local peer configuration.
// Application failures are recorded without discarding the saved result.
func (r *Runtime) RevokeRegistration(
	ctx context.Context,
	networkName string,
	regName string,
) error {
	return execute(r, ctx, func() error {
		if err := r.service.RevokeRegistration(networkName, regName); err != nil {
			return err
		}

		r.applyPeers(networkName)
		return nil
	})
}

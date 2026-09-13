package runtime

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// ListCidrs returns all CIDRs in the given network.
func (r *Runtime) ListCidrs(
	ctx context.Context,
	networkName string,
) (
	[]*service.Cidr,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Cidr, error) {
		return r.service.ListCidrs(networkName)
	})
}

// CreateCidr adds a named CIDR to the network. The CIDR string must parse as a
// valid net.IPNet. The store atomically verifies containment within the
// persisted main CIDR and rejects persisted name or route conflicts.
func (r *Runtime) CreateCidr(
	ctx context.Context,
	networkName string,
	name string,
	cidrStr string,
) error {
	return execute(r, ctx, func() error {
		return r.service.CreateCidr(networkName, name, cidrStr)
	})
}

// UpdateCidr renames a CIDR.
func (r *Runtime) UpdateCidr(
	ctx context.Context,
	networkName string,
	cidrName string,
	newCidrName string,
) error {
	return execute(r, ctx, func() error {
		return r.service.UpdateCidr(networkName, cidrName, newCidrName)
	})
}

// DeleteCidr removes a named CIDR from the network. Any associations
// involving this CIDR are also removed via foreign-key cascades. The
// root CIDR (named after the network) cannot be deleted individually;
// it is removed automatically when the network is deleted.
func (r *Runtime) DeleteCidr(
	ctx context.Context,
	networkName string,
	cidrName string,
) error {
	return execute(r, ctx, func() error {
		return r.service.DeleteCidr(networkName, cidrName)
	})
}

// ListCidrGroups returns the groups directly assigned to a CIDR.
func (r *Runtime) ListCidrGroups(
	ctx context.Context,
	networkName string,
	cidrName string,
) (
	[]*service.Group,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Group, error) {
		return r.service.ListCidrGroups(networkName, cidrName)
	})
}

// AssignCidrGroup assigns a group to a CIDR. This gives the CIDR (and any
// peers belonging to it) the named group.
func (r *Runtime) AssignCidrGroup(
	ctx context.Context,
	network string,
	cidrName string,
	groupName string,
) error {
	return execute(r, ctx, func() error {
		return r.service.AssignCidrGroup(network, cidrName, groupName)
	})
}

// RemoveCidrGroup removes a group assignment from a CIDR.
func (r *Runtime) RemoveCidrGroup(
	ctx context.Context,
	network string,
	cidrName string,
	groupName string,
) error {
	return execute(r, ctx, func() error {
		return r.service.RemoveCidrGroup(network, cidrName, groupName)
	})
}

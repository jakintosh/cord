package runtime

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// ListGroups returns all groups for the given network.
func (r *Runtime) ListGroups(
	ctx context.Context,
	network string,
) (
	[]*service.Group,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Group, error) {
		return r.service.ListGroups(network)
	})
}

// CreateGroup creates a new group with the given name in the network.
func (r *Runtime) CreateGroup(
	ctx context.Context,
	network string,
	name string,
) (
	*service.Group,
	error,
) {
	return executeValue(r, ctx, func() (*service.Group, error) {
		return r.service.CreateGroup(network, name)
	})
}

// DeleteGroup deletes a group by name from the network.
func (r *Runtime) DeleteGroup(
	ctx context.Context,
	network string,
	name string,
) error {
	return execute(r, ctx, func() error {
		return r.service.DeleteGroup(network, name)
	})
}

package runtime

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// ListAssociations returns all group associations in the given network.
func (r *Runtime) ListAssociations(
	ctx context.Context,
	network string,
) (
	[]*service.Association,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Association, error) {
		return r.service.ListAssociations(network)
	})
}

// CreateAssociation creates a group-level association. After the next
// reconciliation, peers in each associated group become visible to
// peers in the other.
func (r *Runtime) CreateAssociation(
	ctx context.Context,
	network string,
	group1 string,
	group2 string,
) error {
	return execute(r, ctx, func() error {
		return r.service.CreateAssociation(network, group1, group2)
	})
}

// DeleteAssociation deletes the association between two groups. After
// the next reconciliation, peers in each group will no longer see
// peers in the other (unless another association connects them).
func (r *Runtime) DeleteAssociation(
	ctx context.Context,
	network string,
	group1 string,
	group2 string,
) error {
	return execute(r, ctx, func() error {
		return r.service.DeleteAssociation(network, group1, group2)
	})
}

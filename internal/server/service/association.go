package service

import "fmt"

// Association declares peer visibility between two groups. Peers whose
// effective groups include Group1 can see peers whose effective groups
// include Group2 and vice versa. Associations are symmetric and stored
// normalized (group1 < group2).
type Association struct {
	Group1 string
	Group2 string
}

// ListAssociations returns all group associations in the given network.
func (s *Service) ListAssociations(
	network string,
) (
	[]*Association,
	error,
) {
	assocs, err := s.store.ListAssociations(network)
	if err != nil {
		return nil, fmt.Errorf("list associations: %w", err)
	}
	return assocs, nil
}

// CreateAssociation creates a group-level association. After the next
// reconciliation, peers in each associated group become visible to
// peers in the other.
func (s *Service) CreateAssociation(
	network string,
	group1 string,
	group2 string,
) error {
	if network == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	if group1 == "" || group2 == "" {
		return fmt.Errorf("%w: group names required", ErrInvalidInput)
	}

	a := &Association{
		Group1: group1,
		Group2: group2,
	}

	if err := s.store.InsertAssociation(network, a); err != nil {
		return fmt.Errorf("insert association: %w", mapStoreError(err))
	}

	return nil
}

// DeleteAssociation deletes the association between two groups. After
// the next reconciliation, peers in each group will no longer see
// peers in the other (unless another association connects them).
func (s *Service) DeleteAssociation(
	network string,
	group1 string,
	group2 string,
) error {
	if err := s.store.DeleteAssociation(network, group1, group2); err != nil {
		return fmt.Errorf("delete association: %w", mapStoreError(err))
	}
	return nil
}

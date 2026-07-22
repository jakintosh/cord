package service

import "fmt"

// Group is a named group that can be assigned to CIDRs. Groups are the
// basis for peer visibility — a peer's effective groups (direct CIDR
// groups + inherited from containing CIDRs) determine which other
// peers it can see through group associations.
type Group struct {
	ID   int64
	Name string
}

// ListGroups returns all groups for the given network.
func (s *Service) ListGroups(
	network string,
) (
	[]*Group,
	error,
) {
	groups, err := s.store.ListGroups(network)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	return groups, nil
}

// CreateGroup creates a new group with the given name in the network.
func (s *Service) CreateGroup(
	network string,
	name string,
) (
	*Group,
	error,
) {
	if network == "" {
		return nil, fmt.Errorf("%w: network name required", ErrInvalidInput)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: group name required", ErrInvalidInput)
	}

	g, err := s.store.InsertGroup(network, name)
	if err != nil {
		return nil, fmt.Errorf("insert group: %w", mapStoreError(err))
	}

	return g, nil
}

// DeleteGroup deletes a group by name from the network.
func (s *Service) DeleteGroup(
	network string,
	name string,
) error {
	if err := s.store.DeleteGroup(network, name); err != nil {
		return fmt.Errorf("delete group: %w", mapStoreError(err))
	}

	return nil
}

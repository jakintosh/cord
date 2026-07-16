package service

import "fmt"

// Assignment links a group to a CIDR. A CIDR's direct groups
// are the groups assigned to it via this relationship.
// Effective groups for a CIDR include both its direct groups
// and groups inherited from containing (ancestor) CIDRs.
type Assignment struct {
	CidrName  string
	GroupName string
}

// ListAssignments returns all group assignments in the given network.
func (s *Service) ListAssignments(
	network string,
) (
	[]*Assignment,
	error,
) {
	assignments, err := s.store.ListAssignments(network)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	return assignments, nil
}

// AssignGroup assigns a group to a CIDR. This gives the CIDR (and any
// peers belonging to it) the named group.
func (s *Service) AssignGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	if network == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}
	if cidrName == "" || groupName == "" {
		return fmt.Errorf("%w: CIDR name and group name required", ErrInvalidInput)
	}

	if err := s.store.AssignGroup(network, cidrName, groupName); err != nil {
		return fmt.Errorf("assign group: %w", mapStoreError(err))
	}
	return nil
}

// RemoveGroup removes a group assignment from a CIDR.
func (s *Service) RemoveGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	if err := s.store.RemoveGroup(network, cidrName, groupName); err != nil {
		return fmt.Errorf("remove group: %w", mapStoreError(err))
	}
	return nil
}

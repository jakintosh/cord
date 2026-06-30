package service

import "fmt"

// Association declares peer visibility between two CIDRs. Peers within
// cidr1 can see peers within cidr2 and vice versa. Associations are
// symmetric and stored normalized (cidr1 < cidr2).
type Association struct {
	Cidr1 string
	Cidr2 string
}

// ListAssociations returns all associations in the given network.
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

// CreateAssociation creates an association between two CIDRs. After the
// next reconciliation, peers in each CIDR become visible to peers in
// the other.
func (s *Service) CreateAssociation(
	network string,
	cidr1 string,
	cidr2 string,
) error {
	if network == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	if cidr1 == "" || cidr2 == "" {
		return fmt.Errorf("%w: CIDR names required", ErrInvalidInput)
	}

	if cidr1 == cidr2 {
		return fmt.Errorf("%w: cannot associate a CIDR with itself", ErrInvalidInput)
	}

	a := &Association{
		Cidr1: cidr1,
		Cidr2: cidr2,
	}

	if err := s.store.InsertAssociation(network, a); err != nil {
		return fmt.Errorf("insert association: %w", mapStoreError(err))
	}

	return nil
}

// DeleteAssociation deletes the association between two CIDRs. After
// the next reconciliation, peers in each CIDR will no longer see
// peers in the other (unless another association connects them).
func (s *Service) DeleteAssociation(
	network string,
	cidr1 string,
	cidr2 string,
) error {
	if err := s.store.DeleteAssociation(network, cidr1, cidr2); err != nil {
		return fmt.Errorf("delete association: %w", mapStoreError(err))
	}
	return nil
}

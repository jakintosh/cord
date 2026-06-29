package service

import (
	"fmt"
	"net"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

// Cidr is a named CIDR range within a server network. CIDRs partition
// the address space for routing and association rules.
type Cidr struct {
	Name   string
	Cidr   string // e.g. "10.42.1.0/24"
	Length int    // prefix length (e.g. 24)
	Prefix int    // total bits (32 for IPv4, 128 for IPv6)
}

// CreateCidrRequest is the input for adding a CIDR to a network.
type CreateCidrRequest struct {
	Name string
	Cidr string
}

// UpdateCidrRequest carries the fields that can be changed on a CIDR.
// Currently only the name is mutable.
type UpdateCidrRequest struct {
	Name string
}

// GetCidr returns a CIDR by name within the given network.
func (s *Service) GetCidr(
	networkName string,
	name string,
) (
	*Cidr,
	error,
) {
	c, err := s.store.GetCidr(networkName, name)
	if err != nil {
		return nil, fmt.Errorf("get cidr %q: %w", name, mapStoreError(err))
	}
	return c, nil
}

// ListCidrs returns all CIDRs in the given network.
func (s *Service) ListCidrs(
	networkName string,
) (
	[]*Cidr,
	error,
) {
	cidrs, err := s.store.ListCidrs(networkName)
	if err != nil {
		return nil, fmt.Errorf("list cidrs: %w", err)
	}
	return cidrs, nil
}

// AddCidr adds a named CIDR to the network. The CIDR string must parse
// as a valid net.IPNet and must be contained within the root CIDR.
// Returns ErrCIDROverlap if the range conflicts with an existing CIDR.
func (s *Service) AddCidr(
	networkName string,
	req CreateCidrRequest,
) error {
	if networkName == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	if req.Name == "" {
		return fmt.Errorf("%w: CIDR name required", ErrInvalidInput)
	}

	_, cidrNet, err := net.ParseCIDR(req.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid CIDR %q: %v", ErrInvalidInput, req.Cidr, err)
	}

	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return fmt.Errorf("get network for cidr check: %w", mapStoreError(err))
	}

	_, rootNet, err := net.ParseCIDR(network.MainCidr)
	if err != nil {
		return fmt.Errorf("%w: parse main CIDR %q: %v", ErrInvalidInput, network.MainCidr, err)
	}

	if !netaddr.Contains(rootNet, cidrNet) {
		return fmt.Errorf(
			"%w: CIDR %q is not contained within main CIDR %q",
			ErrInvalidInput, req.Cidr, network.MainCidr,
		)
	}

	ones, bits := cidrNet.Mask.Size()
	c := &Cidr{
		Name:   req.Name,
		Cidr:   req.Cidr,
		Length: ones,
		Prefix: bits,
	}

	if err := s.store.InsertCidr(networkName, c); err != nil {
		return fmt.Errorf("insert cidr: %w", mapStoreError(err))
	}

	return nil
}

// RemoveCidr removes a named CIDR from the network. Any associations
// involving this CIDR are also removed via foreign-key cascades. The
// root CIDR (named after the network) cannot be deleted individually;
// it is removed automatically when the network is deleted.
func (s *Service) RemoveCidr(
	networkName string,
	name string,
) error {
	if name == networkName {
		return fmt.Errorf("%w: cannot delete the root CIDR", ErrInvalidInput)
	}

	if err := s.store.DeleteCidr(networkName, name); err != nil {
		return fmt.Errorf("delete cidr %q: %w", name, mapStoreError(err))
	}
	return nil
}

// UpdateCidr renames a CIDR and returns the updated record.
func (s *Service) UpdateCidr(
	networkName string,
	name string,
	req UpdateCidrRequest,
) error {
	if req.Name == "" {
		return fmt.Errorf("%w: CIDR name required", ErrInvalidInput)
	}

	_, err := s.store.UpdateCidr(networkName, name, req)
	if err != nil {
		return fmt.Errorf("update cidr %q: %w", name, mapStoreError(err))
	}
	return nil
}

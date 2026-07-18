package service

import (
	"fmt"
	"net"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

// Cidr is a named CIDR range within a server network. CIDRs partition
// the address space for routing and assignment rules. Terminal CIDRs
// represent individual peer addresses and are immutable after creation.
type Cidr struct {
	Name     string
	Cidr     string // e.g. "10.42.1.0/24"
	Prefix   int    // prefix length (e.g. 24)
	Bits     int    // total address bits (32 for IPv4, 128 for IPv6)
	Terminal bool   // whether this is a terminal (single-peer) CIDR
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

// CreateCidr adds a named CIDR to the network. The CIDR string must parse
// as a valid net.IPNet and must be contained within the root CIDR.
// Returns ErrCIDROverlap if the range conflicts with an existing CIDR.
func (s *Service) CreateCidr(
	networkName string,
	name string,
	cidr string,
) error {
	if networkName == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	if name == "" {
		return fmt.Errorf("%w: CIDR name required", ErrInvalidInput)
	}

	_, cidrNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid CIDR %q: %v", ErrInvalidInput, cidr, err)
	}

	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return fmt.Errorf("get network for cidr check: %w", mapStoreError(err))
	}

	_, rootNet, err := net.ParseCIDR(network.Main.Cidr)
	if err != nil {
		return fmt.Errorf("%w: parse main CIDR %q: %v", ErrInvalidInput, network.Main.Cidr, err)
	}

	if !netaddr.Contains(rootNet, cidrNet) {
		return fmt.Errorf(
			"%w: CIDR %q is not contained within main CIDR %q",
			ErrInvalidInput, cidr, network.Main.Cidr,
		)
	}

	ones, bits := cidrNet.Mask.Size()
	c := &Cidr{
		Name:   name,
		Cidr:   cidr,
		Prefix: ones,
		Bits:   bits,
	}

	if err := s.store.InsertCidr(networkName, c); err != nil {
		return fmt.Errorf("insert cidr: %w", mapStoreError(err))
	}

	return nil
}

// UpdateCidr renames a CIDR and returns the updated record.
func (s *Service) UpdateCidr(
	networkName string,
	name string,
	newName string,
) error {
	if newName == "" {
		return fmt.Errorf("%w: CIDR name required", ErrInvalidInput)
	}

	_, err := s.store.UpdateCidr(networkName, name, newName)
	if err != nil {
		return fmt.Errorf("update cidr %q: %w", name, mapStoreError(err))
	}
	return nil
}

// DeleteCidr removes a named CIDR from the network. Any associations
// involving this CIDR are also removed via foreign-key cascades. The
// root CIDR (named after the network) cannot be deleted individually;
// it is removed automatically when the network is deleted.
func (s *Service) DeleteCidr(
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

// ListCidrGroups returns the groups directly assigned to a CIDR.
func (s *Service) ListCidrGroups(
	network string,
	cidrName string,
) (
	[]*Group,
	error,
) {
	if _, err := s.store.GetCidr(network, cidrName); err != nil {
		return nil, fmt.Errorf("get CIDR %q: %w", cidrName, mapStoreError(err))
	}
	groups, err := s.store.ListCidrGroups(network, cidrName)
	if err != nil {
		return nil, fmt.Errorf("list CIDR groups: %w", mapStoreError(err))
	}
	return groups, nil
}

// AssignCidrGroup assigns a group to a CIDR. This gives the CIDR (and any
// peers belonging to it) the named group.
func (s *Service) AssignCidrGroup(
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

	if err := s.store.AssignCidrGroup(network, cidrName, groupName); err != nil {
		return fmt.Errorf("assign group: %w", mapStoreError(err))
	}
	return nil
}

// RemoveCidrGroup removes a group assignment from a CIDR.
func (s *Service) RemoveCidrGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	if _, err := s.store.GetCidr(network, cidrName); err != nil {
		return fmt.Errorf("get CIDR %q: %w", cidrName, mapStoreError(err))
	}
	if err := s.store.RemoveCidrGroup(network, cidrName, groupName); err != nil {
		return fmt.Errorf("remove group: %w", mapStoreError(err))
	}
	return nil
}

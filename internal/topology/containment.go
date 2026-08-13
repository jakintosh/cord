package topology

import (
	"bytes"
	"fmt"
	"net"
	"slices"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

const noParent = -1

// containmentIndex is the canonical CIDR hierarchy shared by the tree and
// resolver. Entries are deterministic and parent-before-child.
type containmentIndex struct {
	entries []containmentEntry
	byName  map[string]int
}

type containmentEntry struct {
	cidr   Cidr
	parent int
	depth  int
}

func (c *containmentIndex) index(
	name string,
) (
	int,
	bool,
) {
	index, ok := c.byName[name]
	return index, ok
}

func (c *containmentIndex) entry(
	name string,
) (
	containmentEntry,
	bool,
) {
	index, ok := c.index(name)
	if !ok {
		return containmentEntry{}, false
	}
	return c.entries[index], true
}

func (c *containmentIndex) ancestryIndexes(
	name string,
) (
	[]int,
	bool,
) {
	index, ok := c.index(name)
	if !ok {
		return nil, false
	}

	ancestry := make([]int, 0, c.entries[index].depth+1)
	for index != noParent {
		ancestry = append(ancestry, index)
		index = c.entries[index].parent
	}
	return ancestry, true
}

func (c *containmentIndex) ancestryNames(
	name string,
) (
	stringSet,
	bool,
) {
	indexes, ok := c.ancestryIndexes(name)
	if !ok {
		return nil, false
	}

	names := make(stringSet, len(indexes))
	for _, index := range indexes {
		names.add(c.entries[index].cidr.Name)
	}
	return names, true
}

func (c *containmentIndex) nearestAncestorName(
	entry containmentEntry,
	include func(string) bool,
) string {
	for parent := entry.parent; parent != noParent; parent = c.entries[parent].parent {
		name := c.entries[parent].cidr.Name
		if include(name) {
			return name
		}
	}
	return ""
}

func buildContainment(
	cidrs []Cidr,
) (
	*containmentIndex,
	error,
) {
	ordered := make([]Cidr, len(cidrs))
	seenNames := make(map[string]bool, len(cidrs))

	// Normalize once so later ordering and containment checks can compare bytes.
	for i := range cidrs {
		cidr, err := normalizeCidrInfo(cidrs[i])
		if err != nil {
			return nil, err
		}
		if seenNames[cidr.Name] {
			return nil, fmt.Errorf("duplicate CIDR name %q", cidr.Name)
		}
		seenNames[cidr.Name] = true
		ordered[i] = cidr
	}

	// Base-address order keeps descendants together. When bases match, the
	// shorter prefix comes first so a parent always precedes its children.
	sortCidrs(ordered)

	// Equal ranges are adjacent after sorting, so reject them in one pass.
	for i := 1; i < len(ordered); i++ {
		previous := ordered[i-1]
		current := ordered[i]
		if previous.Bits == current.Bits &&
			previous.Prefix == current.Prefix &&
			bytes.Equal(previous.Base, current.Base) {
			return nil, fmt.Errorf(
				"CIDRs %q and %q have the same range",
				previous.Name,
				current.Name,
			)
		}
	}

	// The stack holds the open ancestor chain at the current address, root first.
	stack := make([]int, 0, len(ordered))
	byName := make(map[string]int, len(ordered))
	entries := make([]containmentEntry, 0, len(ordered))
	for _, cidr := range ordered {
		// Pop completed or disjoint branches until the nearest parent is on top.
		for len(stack) > 0 {
			parent := entries[stack[len(stack)-1]].cidr
			if parent.contains(cidr) {
				break
			}
			stack = stack[:len(stack)-1]
		}

		parent := noParent
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}

		entryIndex := len(entries)
		entries = append(entries, containmentEntry{
			cidr:   cidr,
			parent: parent,
			depth:  len(stack),
		})

		byName[cidr.Name] = entryIndex
		stack = append(stack, entryIndex)
	}

	return &containmentIndex{
		entries: entries,
		byName:  byName,
	}, nil
}

func normalizeCidrInfo(
	cidr Cidr,
) (
	Cidr,
	error,
) {
	if cidr.Name == "" {
		return Cidr{}, fmt.Errorf("CIDR name is required")
	}
	network, err := netaddr.ParseNetworkCIDR(cidr.Cidr)
	if err != nil {
		return Cidr{}, fmt.Errorf("parse CIDR %q: %w", cidr.Cidr, err)
	}
	parsedPrefix, parsedBits := network.Mask.Size()

	if cidr.Bits != 32 && cidr.Bits != 128 {
		return Cidr{}, fmt.Errorf(
			"CIDR %q has invalid address length %d",
			cidr.Name,
			cidr.Bits,
		)
	}

	if cidr.Prefix < 0 || cidr.Prefix > cidr.Bits {
		return Cidr{}, fmt.Errorf(
			"CIDR %q has invalid prefix %d for address length %d",
			cidr.Name,
			cidr.Prefix,
			cidr.Bits,
		)
	}
	if cidr.Prefix != parsedPrefix || cidr.Bits != parsedBits {
		return Cidr{}, fmt.Errorf(
			"CIDR %q metadata does not match %q",
			cidr.Name,
			cidr.Cidr,
		)
	}

	base := normalizeIP(cidr.Base, cidr.Bits)
	last := normalizeIP(cidr.Last, cidr.Bits)
	if base == nil || last == nil {
		return Cidr{}, fmt.Errorf(
			"CIDR %q has addresses inconsistent with address length %d",
			cidr.Name,
			cidr.Bits,
		)
	}

	metadataNetwork := net.IPNet{
		IP:   base,
		Mask: net.CIDRMask(cidr.Prefix, cidr.Bits),
	}
	expectedBase, expectedLast := netaddr.Range(&metadataNetwork)
	parsedBase, parsedLast := netaddr.Range(network)

	if !bytes.Equal(base, expectedBase) || !bytes.Equal(last, expectedLast) {
		return Cidr{}, fmt.Errorf(
			"CIDR %q has inconsistent range for /%d",
			cidr.Name,
			cidr.Prefix,
		)
	}
	if !bytes.Equal(base, parsedBase) || !bytes.Equal(last, parsedLast) {
		return Cidr{}, fmt.Errorf(
			"CIDR %q range does not match %q",
			cidr.Name,
			cidr.Cidr,
		)
	}

	cidr.Cidr = network.String()
	cidr.Base = base
	cidr.Last = last
	return cidr, nil
}

func normalizeIP(
	ip net.IP,
	bits int,
) net.IP {
	ip = netaddr.Normalize(ip)
	if len(ip)*8 != bits {
		return nil
	}
	return slices.Clone(ip)
}

package topology

import (
	"bytes"
	"cmp"
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
	slices.SortFunc(ordered, func(a, b Cidr) int {
		if n := cmp.Compare(a.Bits, b.Bits); n != 0 {
			return n
		}
		if n := bytes.Compare(a.Base, b.Base); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Prefix, b.Prefix); n != 0 {
			return n
		}
		return cmp.Compare(a.Name, b.Name)
	})

	// ensure no duplicate ranges
	// equal ranges are adjacent after sorting
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
			if containsCidr(parent, cidr) {
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

	base := normalizeIP(cidr.Base, cidr.Bits)
	last := normalizeIP(cidr.Last, cidr.Bits)
	if base == nil || last == nil {
		return Cidr{}, fmt.Errorf(
			"CIDR %q has addresses inconsistent with address length %d",
			cidr.Name,
			cidr.Bits,
		)
	}

	network := net.IPNet{
		IP:   base,
		Mask: net.CIDRMask(cidr.Prefix, cidr.Bits),
	}
	expectedBase, expectedLast := netaddr.Range(&network)

	if !bytes.Equal(base, expectedBase) || !bytes.Equal(last, expectedLast) {
		return Cidr{}, fmt.Errorf(
			"CIDR %q has inconsistent range for /%d",
			cidr.Name,
			cidr.Prefix,
		)
	}

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

func containsCidr(
	outer Cidr,
	inner Cidr,
) bool {
	return outer.Bits == inner.Bits &&
		bytes.Compare(outer.Base, inner.Base) <= 0 &&
		bytes.Compare(outer.Last, inner.Last) >= 0
}

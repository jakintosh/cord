package topology

import (
	"bytes"
	"cmp"
	"slices"
)

// compareCidrs defines the canonical topology order. IPv4 sorts before IPv6,
// then ranges sort by base address, prefix length, and finally name.
func compareCidrs(
	a Cidr,
	b Cidr,
) int {
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
}

func sortCidrs(
	cidrs []Cidr,
) {
	slices.SortFunc(cidrs, compareCidrs)
}

func compareAssociations(
	a Association,
	b Association,
) int {
	if n := cmp.Compare(a.Group1, b.Group1); n != 0 {
		return n
	}
	return cmp.Compare(a.Group2, b.Group2)
}

package topology

import (
	"fmt"
	"slices"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

// Association is one normalized group-to-group relationship. Group1 is
// lexically less than or equal to Group2.
type Association struct {
	Group1 string
	Group2 string
}

// View is a complete display projection of a topology. DisplayParent names
// the nearest disclosed containing CIDR, which need not be the node's actual
// immediate parent in a projected view.
type View struct {
	Nodes           []ViewNode
	Associations    []Association
	EffectiveGroups []string
	SubjectPeer     string
}

// ViewNode is one disclosed CIDR and its display relationship.
type ViewNode struct {
	Cidr          Cidr
	DisplayParent string
	Groups        []string
	PeerName      string
	Subject       bool
}

// CidrFromString constructs canonical topology CIDR metadata from its public
// string representation.
func CidrFromString(
	name string,
	cidr string,
	terminal bool,
) (
	Cidr,
	error,
) {
	network, err := netaddr.ParseNetworkCIDR(cidr)
	if err != nil {
		return Cidr{}, fmt.Errorf("parse CIDR %q: %w", cidr, err)
	}
	prefix, bits := network.Mask.Size()
	base, last := netaddr.Range(network)
	return normalizeCidrInfo(Cidr{
		Name:     name,
		Cidr:     network.String(),
		Base:     base,
		Last:     last,
		Prefix:   prefix,
		Bits:     bits,
		Terminal: terminal,
	})
}

// NormalizeView validates a view and returns a deterministic copy.
func NormalizeView(
	view View,
) (
	View,
	error,
) {
	nodes, err := normalizeViewNodes(view.Nodes, view.SubjectPeer)
	if err != nil {
		return View{}, err
	}
	associations, err := normalizeAssociations(view.Associations)
	if err != nil {
		return View{}, err
	}

	return View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: normalizeStrings(view.EffectiveGroups),
		SubjectPeer:     view.SubjectPeer,
	}, nil
}

func normalizeViewNodes(
	viewNodes []ViewNode,
	subjectPeer string,
) (
	[]ViewNode,
	error,
) {
	nodes := make([]ViewNode, len(viewNodes))
	byName := make(map[string]Cidr, len(viewNodes))
	subjects := 0
	for i, node := range viewNodes {
		cidr, err := normalizeCidrInfo(node.Cidr)
		if err != nil {
			return nil, err
		}
		if _, exists := byName[cidr.Name]; exists {
			return nil, fmt.Errorf("duplicate view CIDR name %q", cidr.Name)
		}
		byName[cidr.Name] = cidr

		if node.Subject {
			subjects++
		}
		nodes[i] = ViewNode{
			Cidr:          cidr,
			DisplayParent: node.DisplayParent,
			Groups:        normalizeStrings(node.Groups),
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
	}

	for _, node := range nodes {
		if node.DisplayParent == "" {
			continue
		}
		if node.DisplayParent == node.Cidr.Name {
			return nil, fmt.Errorf("view CIDR %q is its own display parent", node.Cidr.Name)
		}
		parent, ok := byName[node.DisplayParent]
		if !ok {
			return nil, fmt.Errorf(
				"view CIDR %q has missing display parent %q",
				node.Cidr.Name,
				node.DisplayParent,
			)
		}
		if !parent.contains(node.Cidr) {
			return nil, fmt.Errorf(
				"view CIDR %q is not contained by display parent %q",
				node.Cidr.Name,
				node.DisplayParent,
			)
		}
	}
	if subjects > 1 {
		return nil, fmt.Errorf("view has multiple subject CIDRs")
	}
	if subjectPeer != "" && subjects != 1 {
		return nil, fmt.Errorf("projected view must have exactly one subject CIDR")
	}

	slices.SortFunc(nodes, func(a, b ViewNode) int {
		return compareCidrs(a.Cidr, b.Cidr)
	})
	for index := 1; index < len(nodes); index++ {
		previous := nodes[index-1].Cidr
		current := nodes[index].Cidr
		if previous.Bits == current.Bits &&
			previous.Prefix == current.Prefix &&
			previous.Base.Equal(current.Base) {
			return nil, fmt.Errorf(
				"view CIDRs %q and %q have the same range",
				previous.Name,
				current.Name,
			)
		}
	}
	if err := validateDisplayParents(nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func normalizeAssociations(
	viewAssociations []Association,
) (
	[]Association,
	error,
) {
	associations := make([]Association, len(viewAssociations))
	for i, association := range viewAssociations {
		if association.Group1 == "" || association.Group2 == "" {
			return nil, fmt.Errorf("association groups are required")
		}
		associations[i] = normalizeAssociation(
			association.Group1,
			association.Group2,
		)
	}
	slices.SortFunc(associations, compareAssociations)
	return slices.Compact(associations), nil
}

func normalizeStrings(
	values []string,
) []string {
	result := slices.Clone(values)
	if result == nil {
		result = []string{}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func validateDisplayParents(
	nodes []ViewNode,
) error {
	parents := make(map[string]string, len(nodes))
	for _, node := range nodes {
		parents[node.Cidr.Name] = node.DisplayParent
	}
	for _, node := range nodes {
		seen := make(map[string]bool)
		for name := node.Cidr.Name; name != ""; name = parents[name] {
			if seen[name] {
				return fmt.Errorf("view display parents contain a cycle at %q", name)
			}
			seen[name] = true
		}
	}
	return nil
}

func normalizeAssociation(
	group1 string,
	group2 string,
) Association {
	if group2 < group1 {
		group1, group2 = group2, group1
	}
	return Association{Group1: group1, Group2: group2}
}

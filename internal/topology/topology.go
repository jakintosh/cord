package topology

import (
	"fmt"
	"slices"
)

// Topology is a compiled snapshot whose tree and resolver share one
// containment index.
type Topology struct {
	tree     *Tree
	resolver *Resolver
}

type disclosure struct {
	cidrs           stringSet
	subjectAncestry stringSet
}

// New compiles a snapshot for hierarchy inspection and visibility resolution.
func New(
	s *Snapshot,
) (
	*Topology,
	error,
) {
	if err := validateSnapshot(s); err != nil {
		return nil, err
	}
	containment, err := buildContainment(s.Cidrs)
	if err != nil {
		return nil, err
	}

	return &Topology{
		tree:     newTree(s, containment),
		resolver: newResolver(s, containment),
	}, nil
}

func (t *Topology) Tree() *Tree {
	return t.tree
}

func (t *Topology) Resolver() *Resolver {
	return t.resolver
}

// FullView returns every CIDR, direct assignment, peer, and association.
func (t *Topology) FullView() View {
	view, err := NormalizeView(t.buildView(nil, Visibility{}))
	if err != nil {
		panic(fmt.Sprintf("build full topology view: %v", err))
	}
	return view
}

// ProjectedView returns the topology disclosed to peerName. Undisclosed
// ancestors are contracted and leave no marker in the returned view.
func (t *Topology) ProjectedView(
	peerName string,
) (
	View,
	error,
) {
	visibility, err := t.resolver.Visibility(peerName)
	if err != nil {
		return View{}, err
	}
	return t.ProjectedViewFromVisibility(visibility)
}

// ProjectedViewFromVisibility constructs a projected display view from a
// visibility result produced by this compiled topology.
func (t *Topology) ProjectedViewFromVisibility(
	visibility Visibility,
) (
	View,
	error,
) {
	disclosure, ok := t.disclosure(visibility)
	if !ok {
		return View{}, fmt.Errorf("subject CIDR %q not found", visibility.SubjectCidr)
	}
	return NormalizeView(t.buildView(&disclosure, visibility))
}

func (t *Topology) buildView(
	disclosure *disclosure,
	visibility Visibility,
) View {
	full := disclosure == nil
	peerByCidr := t.peerByCidr()

	var nodes []ViewNode
	for _, entry := range t.resolver.containment.entries {
		if !full && !disclosure.cidrs.contains(entry.cidr.Name) {
			continue
		}
		nodes = append(nodes, t.viewNode(entry, peerByCidr, disclosure, visibility))
	}

	associations := visibility.Associations
	effectiveGroups := visibility.EffectiveGroups
	if full {
		associations = associationsFromMap(t.resolver.associations)
		effectiveGroups = nil
	}

	return View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: effectiveGroups,
		SubjectPeer:     visibility.SubjectPeer,
	}
}

func (t *Topology) disclosure(
	visibility Visibility,
) (
	disclosure,
	bool,
) {
	subjectAncestry, ok := t.resolver.containment.ancestryNames(visibility.SubjectCidr)
	if !ok {
		return disclosure{}, false
	}

	reachableGroups := stringSetFromBoolMap(visibility.ReachableGroups)
	disclosed := subjectAncestry.clone()
	for index, groups := range t.resolver.effectiveGroups {
		if groups.intersects(reachableGroups) {
			disclosed.add(t.resolver.containment.entries[index].cidr.Name)
		}
	}

	return disclosure{
		cidrs:           disclosed,
		subjectAncestry: subjectAncestry,
	}, true
}

func (t *Topology) peerByCidr() map[string]string {
	result := make(map[string]string, len(t.resolver.peerCidr))
	for peer, cidr := range t.resolver.peerCidr {
		result[cidr] = peer
	}
	return result
}

func (t *Topology) viewNode(
	entry containmentEntry,
	peerByCidr map[string]string,
	disclosure *disclosure,
	visibility Visibility,
) ViewNode {
	groups := slices.Clone(t.tree.nodes[entry.cidr.Name].Groups)
	if disclosure != nil && !disclosure.subjectAncestry.contains(entry.cidr.Name) {
		groups = slices.DeleteFunc(groups, func(group string) bool {
			return !visibility.ReachableGroups[group]
		})
	}

	displayParent := t.resolver.containment.nearestAncestorName(
		entry,
		func(name string) bool {
			return disclosure == nil || disclosure.cidrs.contains(name)
		},
	)

	return ViewNode{
		Cidr:          entry.cidr,
		DisplayParent: displayParent,
		Groups:        groups,
		PeerName:      peerByCidr[entry.cidr.Name],
		Subject:       entry.cidr.Name == visibility.SubjectCidr,
	}
}

func associationsFromMap(
	associations map[string]stringSet,
) []Association {
	set := make(map[Association]struct{})
	for group1, targets := range associations {
		for group2 := range targets {
			set[normalizeAssociation(group1, group2)] = struct{}{}
		}
	}

	result := make([]Association, 0, len(set))
	for association := range set {
		result = append(result, association)
	}
	slices.SortFunc(result, compareAssociations)
	return result
}

func validateSnapshot(
	s *Snapshot,
) error {
	if s == nil {
		return fmt.Errorf("topology snapshot is required")
	}

	cidrNames := make(stringSet, len(s.Cidrs))
	for _, cidr := range s.Cidrs {
		cidrNames.add(cidr.Name)
	}
	if err := validateAssignments(s.Assignments, cidrNames); err != nil {
		return err
	}
	if err := validateAssociations(s.Associations); err != nil {
		return err
	}
	return validatePeers(s.PeerCidr, s.PeerInfo, cidrNames)
}

func validateAssignments(
	assignments map[string][]string,
	cidrNames stringSet,
) error {
	for cidrName, groups := range assignments {
		if !cidrNames.contains(cidrName) {
			return fmt.Errorf("assignment CIDR %q not found", cidrName)
		}
		for _, group := range groups {
			if group == "" {
				return fmt.Errorf("assignment group for CIDR %q is required", cidrName)
			}
		}
	}
	return nil
}

func validateAssociations(
	associations map[string]map[string]bool,
) error {
	for source, targets := range associations {
		if source == "" {
			return fmt.Errorf("association source group is required")
		}
		for target, associated := range targets {
			if associated && target == "" {
				return fmt.Errorf("association target group is required")
			}
		}
	}
	return nil
}

func validatePeers(
	peerCidr map[string]string,
	peerInfo map[string]Peer,
	cidrNames stringSet,
) error {
	peerByCidr := make(map[string]string, len(peerCidr))
	for peerName, cidrName := range peerCidr {
		if !cidrNames.contains(cidrName) {
			return fmt.Errorf("peer %q CIDR %q not found", peerName, cidrName)
		}
		peer, ok := peerInfo[peerName]
		if !ok {
			return fmt.Errorf("peer %q information not found", peerName)
		}
		if peer.Name != peerName {
			return fmt.Errorf("peer %q information has name %q", peerName, peer.Name)
		}
		if otherPeer, exists := peerByCidr[cidrName]; exists {
			return fmt.Errorf(
				"peers %q and %q share CIDR %q",
				otherPeer,
				peerName,
				cidrName,
			)
		}
		peerByCidr[cidrName] = peerName
	}

	for peerName := range peerInfo {
		if _, ok := peerCidr[peerName]; !ok {
			return fmt.Errorf("peer %q CIDR assignment not found", peerName)
		}
	}
	return nil
}

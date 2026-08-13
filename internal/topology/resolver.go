package topology

import (
	"fmt"
	"slices"
)

type Resolver struct {
	containment     *containmentIndex
	effectiveGroups []stringSet
	associations    map[string]stringSet
	peerCidr        map[string]string
	peerInfo        map[string]Peer
}

// Visibility explains the group paths and peers visible from one peer.
type Visibility struct {
	SubjectPeer     string
	SubjectCidr     string
	EffectiveGroups []string
	ReachableGroups map[string]bool
	Associations    []Association
	Peers           []Peer
}

// NewResolver compiles a snapshot for visibility resolution.
func NewResolver(
	s *Snapshot,
) (
	*Resolver,
	error,
) {
	if err := validateSnapshot(s); err != nil {
		return nil, err
	}
	containment, err := buildContainment(s.Cidrs)
	if err != nil {
		return nil, err
	}
	return newResolver(s, containment), nil
}

// GetEffectiveGroups returns all direct and inherited groups for cidrName.
func (r *Resolver) GetEffectiveGroups(
	cidrName string,
) (
	map[string]bool,
	error,
) {
	index, ok := r.containment.index(cidrName)
	if !ok {
		return nil, fmt.Errorf("cidr %q not found", cidrName)
	}
	return r.effectiveGroups[index].boolMap(), nil
}

// GetPeerCIDR returns the CIDR name assigned to peerName.
func (r *Resolver) GetPeerCIDR(
	peerName string,
) (
	string,
	bool,
) {
	cidrName, ok := r.peerCidr[peerName]
	return cidrName, ok
}

// VisiblePeers returns the peers visible from peerName.
func (r *Resolver) VisiblePeers(
	peerName string,
) (
	[]Peer,
	error,
) {
	visibility, err := r.Visibility(peerName)
	return visibility.Peers, err
}

// Visibility returns the complete visibility explanation for peerName.
func (r *Resolver) Visibility(
	peerName string,
) (
	Visibility,
	error,
) {
	cidrName, ok := r.peerCidr[peerName]
	if !ok {
		return Visibility{}, fmt.Errorf("peer %q not found", peerName)
	}

	effectiveGroups := r.groupsForCidr(cidrName)
	reachableGroups := r.reachableGroups(effectiveGroups)
	usedTargetGroups := r.usedTargetGroups(cidrName, reachableGroups)

	return Visibility{
		SubjectPeer:     peerName,
		SubjectCidr:     cidrName,
		EffectiveGroups: effectiveGroups.sorted(),
		ReachableGroups: reachableGroups.boolMap(),
		Associations:    r.visibleAssociations(effectiveGroups, usedTargetGroups),
		Peers:           r.visiblePeers(peerName, reachableGroups),
	}, nil
}

func newResolver(
	s *Snapshot,
	containment *containmentIndex,
) *Resolver {
	return &Resolver{
		containment:     containment,
		effectiveGroups: compileEffectiveGroups(s.Assignments, containment),
		associations:    compileAssociations(s.Associations),
		peerCidr:        cloneStringMap(s.PeerCidr),
		peerInfo:        clonePeerMap(s.PeerInfo),
	}
}

func (r *Resolver) groupsForCidr(
	cidrName string,
) stringSet {
	index, ok := r.containment.index(cidrName)
	if !ok {
		return nil
	}
	return r.effectiveGroups[index]
}

func (r *Resolver) reachableGroups(
	effectiveGroups stringSet,
) stringSet {
	reachable := make(stringSet)
	for source := range effectiveGroups {
		for target := range r.associations[source] {
			reachable.add(target)
		}
	}
	return reachable
}

func (r *Resolver) usedTargetGroups(
	cidrName string,
	reachableGroups stringSet,
) stringSet {
	ancestry, _ := r.containment.ancestryIndexes(cidrName)
	subjectAncestry := make(map[int]bool, len(ancestry))
	for _, index := range ancestry {
		subjectAncestry[index] = true
	}

	used := make(stringSet)
	for index, groups := range r.effectiveGroups {
		if subjectAncestry[index] {
			continue
		}
		for group := range groups {
			if reachableGroups.contains(group) {
				used.add(group)
			}
		}
	}
	return used
}

func (r *Resolver) visiblePeers(
	peerName string,
	reachableGroups stringSet,
) []Peer {
	var peers []Peer
	for otherPeer, otherCidr := range r.peerCidr {
		if otherPeer == peerName {
			continue
		}
		if !r.groupsForCidr(otherCidr).intersects(reachableGroups) {
			continue
		}
		peers = append(peers, r.peerInfo[otherPeer])
	}
	slices.SortFunc(peers, r.comparePeersByCidr)
	return peers
}

func (r *Resolver) visibleAssociations(
	effectiveGroups stringSet,
	usedTargetGroups stringSet,
) []Association {
	set := make(map[Association]struct{})
	for source := range effectiveGroups {
		for target := range r.associations[source] {
			if usedTargetGroups.contains(target) {
				set[normalizeAssociation(source, target)] = struct{}{}
			}
		}
	}

	associations := make([]Association, 0, len(set))
	for association := range set {
		associations = append(associations, association)
	}
	slices.SortFunc(associations, compareAssociations)
	return associations
}

func (r *Resolver) comparePeersByCidr(
	a Peer,
	b Peer,
) int {
	aEntry, _ := r.containment.entry(r.peerCidr[a.Name])
	bEntry, _ := r.containment.entry(r.peerCidr[b.Name])

	return compareCidrs(aEntry.cidr, bEntry.cidr)
}

func compileEffectiveGroups(
	assignments map[string][]string,
	containment *containmentIndex,
) []stringSet {
	effectiveGroups := make([]stringSet, len(containment.entries))
	for index, entry := range containment.entries {
		groups := newStringSet(assignments[entry.cidr.Name]...)
		if entry.parent != noParent {
			for group := range effectiveGroups[entry.parent] {
				groups.add(group)
			}
		}
		effectiveGroups[index] = groups
	}
	return effectiveGroups
}

func compileAssociations(
	associations map[string]map[string]bool,
) map[string]stringSet {
	result := make(map[string]stringSet, len(associations))
	for source, targets := range associations {
		compiledTargets := make(stringSet)
		for target, associated := range targets {
			if associated {
				compiledTargets.add(target)
			}
		}
		result[source] = compiledTargets
	}
	return result
}

func cloneStringMap(
	source map[string]string,
) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func clonePeerMap(
	source map[string]Peer,
) map[string]Peer {
	result := make(map[string]Peer, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

package topology

import "fmt"

type Resolver struct {
	containment     *containmentIndex
	effectiveGroups []map[string]bool
	associations    map[string]map[string]bool
	peerCidr        map[string]string
	peerInfo        map[string]Peer
}

func newResolver(
	s *Snapshot,
	containment *containmentIndex,
) *Resolver {
	effectiveGroups := make(
		[]map[string]bool,
		len(containment.entries),
	)
	for i, entry := range containment.entries {
		groups := make(map[string]bool, len(s.Assignments[entry.cidr.Name]))
		for _, group := range s.Assignments[entry.cidr.Name] {
			groups[group] = true
		}
		if entry.parent != noParent {
			for group := range effectiveGroups[entry.parent] {
				groups[group] = true
			}
		}
		effectiveGroups[i] = groups
	}

	return &Resolver{
		containment:     containment,
		effectiveGroups: effectiveGroups,
		associations:    s.Associations,
		peerCidr:        s.PeerCidr,
		peerInfo:        s.PeerInfo,
	}
}

func NewResolver(
	s *Snapshot,
) (
	*Resolver,
	error,
) {
	containment, err := buildContainment(s.Cidrs)
	if err != nil {
		return nil, err
	}
	return newResolver(s, containment), nil
}
func (r *Resolver) GetEffectiveGroups(
	cidrName string,
) (
	map[string]bool,
	error,
) {
	index, ok := r.containment.byName[cidrName]
	if !ok {
		return nil, fmt.Errorf("cidr %q not found", cidrName)
	}
	return r.effectiveGroups[index], nil
}

func (r *Resolver) GetPeerCIDR(
	peerName string,
) (
	string,
	bool,
) {
	cidrName, ok := r.peerCidr[peerName]
	return cidrName, ok
}

func (r *Resolver) VisiblePeers(
	peerName string,
) (
	[]Peer,
	error,
) {
	cidrName, ok := r.peerCidr[peerName]
	if !ok {
		return nil, fmt.Errorf("peer %q not found", peerName)
	}

	effectiveGroups := r.groupsForCidr(cidrName)
	visibleGroups := make(map[string]bool)
	for group := range effectiveGroups {
		for target := range r.associations[group] {
			visibleGroups[target] = true
		}
	}

	var result []Peer
	for otherPeer, otherCidr := range r.peerCidr {
		if otherPeer == peerName {
			continue
		}
		otherGroups := r.groupsForCidr(otherCidr)
		if intersect(otherGroups, visibleGroups) {
			info := r.peerInfo[otherPeer]
			result = append(result, info)
		}
	}

	return result, nil
}

func (r *Resolver) groupsForCidr(
	cidrName string,
) map[string]bool {
	index, ok := r.containment.byName[cidrName]
	if !ok {
		return nil
	}
	return r.effectiveGroups[index]
}

func intersect(
	a map[string]bool,
	b map[string]bool,
) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

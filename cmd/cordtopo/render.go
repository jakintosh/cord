package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func renderHeading(
	heading string,
) {
	fmt.Println(heading)
	fmt.Println(strings.Repeat("-", len(heading)))
}

func renderPeerAncestryAndGroups(
	w io.Writer,
	cidrName string,
	effGroups map[string]bool,
	tree *topology.Tree,
) {
	tree.PrintAncestry(w, cidrName)

	groups := make([]string, 0, len(effGroups))
	for g := range effGroups {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	groupStr := strings.Join(groups, ", ")
	fmt.Fprintf(w, "\neffective groups: [%s]\n", groupStr)
}

func renderAllAssociations(
	associations []Association,
	effGroups map[string]bool,
) {
	shown := make(map[string]bool)
	for _, a := range associations {
		g1, g2 := a.Group1, a.Group2
		if !effGroups[g1] && !effGroups[g2] {
			continue
		}
		if !effGroups[g1] {
			g1, g2 = g2, g1
		}
		key := g1 + "<->" + g2
		if shown[key] {
			continue
		}
		shown[key] = true
		printAssoc(g1, g2)
	}
}

func renderConnectingPaths(
	associations []Association,
	eff1 map[string]bool,
	eff2 map[string]bool,
) bool {
	shown := make(map[string]bool)
	found := false
	for g1 := range eff1 {
		for _, a := range associations {
			var left, right string
			if a.Group1 == g1 && eff2[a.Group2] {
				left, right = g1, a.Group2
			} else if a.Group2 == g1 && eff2[a.Group1] {
				left, right = g1, a.Group1
			} else {
				continue
			}
			key := left + "<->" + right
			if shown[key] {
				continue
			}
			shown[key] = true
			printAssoc(left, right)
			found = true
		}
	}
	if !found {
		fmt.Println("(no shared associated groups)")
	}
	return found
}

func renderVisiblePeers(
	visible []topology.Peer,
	visibleGroups map[string]bool,
	resolver *topology.Resolver,
) {
	if len(visible) == 0 {
		fmt.Println("(none)")
		return
	}

	byGroup := make(map[string][]topology.Peer)
	for _, p := range visible {
		cidrName, ok := resolver.GetPeerCIDR(p.Name)
		if !ok {
			continue
		}
		peerGroups, err := resolver.GetEffectiveGroups(cidrName)
		if err != nil {
			continue
		}
		for g := range peerGroups {
			if visibleGroups[g] {
				byGroup[g] = append(byGroup[g], p)
			}
		}
	}

	groupNames := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groupNames = append(groupNames, g)
	}
	sort.Strings(groupNames)

	for _, g := range groupNames {
		peers := byGroup[g]
		sort.Slice(peers, func(i, j int) bool {
			return peers[i].Name < peers[j].Name
		})
		fmt.Println(g)
		for _, p := range peers {
			fmt.Printf("- %s | %s\n", p.Route, p.Name)
		}
	}
}

func printAssoc(
	left string,
	right string,
) {
	suffix := ""
	if left == right {
		suffix = "    (self-association)"
	}
	fmt.Printf("%s <-> %s%s\n", left, right, suffix)
}

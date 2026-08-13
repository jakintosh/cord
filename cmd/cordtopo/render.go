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

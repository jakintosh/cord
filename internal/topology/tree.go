package topology

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

type Tree struct {
	nodes map[string]*TreeNode
	roots []*TreeNode
}

type TreeNode struct {
	Cidr     Cidr
	Parent   *TreeNode
	Children []*TreeNode
	Depth    int
	Groups   []string
}

func NewTree(
	s *Snapshot,
) (
	*Tree,
	error,
) {
	containment, err := buildContainment(s.Cidrs)
	if err != nil {
		return nil, err
	}
	return newTree(s, containment), nil
}

func newTree(
	s *Snapshot,
	containment *containmentIndex,
) *Tree {
	nodes := make(map[string]*TreeNode, len(containment.entries))
	ordered := make([]*TreeNode, len(containment.entries))
	var roots []*TreeNode

	for i, entry := range containment.entries {
		groups := slices.Clone(s.Assignments[entry.cidr.Name])
		if groups == nil {
			groups = []string{}
		}
		node := &TreeNode{
			Cidr:   entry.cidr,
			Depth:  entry.depth,
			Groups: groups,
		}
		ordered[i] = node
		nodes[node.Cidr.Name] = node

		if entry.parent == noParent {
			roots = append(roots, node)
			continue
		}

		parent := ordered[entry.parent]
		node.Parent = parent
		parent.Children = append(parent.Children, node)
	}

	return &Tree{
		nodes: nodes,
		roots: roots,
	}
}

func (t *Tree) Node(
	name string,
) *TreeNode {
	return t.nodes[name]
}

func (t *Tree) Nodes() map[string]*TreeNode {
	return t.nodes
}

func (t *Tree) PrintAncestry(
	w io.Writer,
	cidrName string,
) {
	node := t.nodes[cidrName]
	if node == nil {
		return
	}

	var lineage []*TreeNode
	for cur := node; cur != nil; cur = cur.Parent {
		lineage = append(lineage, cur)
	}

	for _, n := range slices.Backward(lineage) {
		prefix := strings.Repeat("-", n.Depth)
		if n.Depth > 0 {
			prefix += " "
		}
		groups := ""
		if len(n.Groups) > 0 {
			groups = " [" + strings.Join(n.Groups, ", ") + "]"
		}
		fmt.Fprintf(w, "%s%s => %s%s\n", prefix, n.Cidr.Cidr, n.Cidr.Name, groups)
	}
}

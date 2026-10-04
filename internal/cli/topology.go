package cli

import (
	"fmt"
	"io"
	"strings"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// TopologyOptions supplies presentation context for a compiled topology.
type TopologyOptions struct {
	Heading     string
	Metadata    string
	BoldSubject bool
	Connected   map[string]bool
}

type topologyPresentation struct {
	options  TopologyOptions
	children map[string][]topology.ViewNode
}

// Topology presents a compiled view, shared by cord and the cordtopo tool.
func (c *CLI) Topology(
	view topology.View,
	opts TopologyOptions,
) error {
	if c.format == FormatJSON {
		return c.JSON(view)
	}

	view, err := topology.NormalizeView(view)
	if err != nil {
		return err
	}

	tree := topologyTree(view, opts)

	var out strings.Builder
	heading := render.Clean(opts.Heading)
	if heading == "" {
		heading = "topology"
	}
	fmt.Fprintf(&out, "%s\n%s\n", heading, strings.Repeat("-", len(heading)))

	if opts.Metadata != "" {
		fmt.Fprintln(&out, render.Clean(opts.Metadata))
	}

	if err := tree.Render(&out, render.Options{
		Style: c.style,
	}); err != nil {
		return err
	}

	if len(view.EffectiveGroups) > 0 {
		groups := make([]string, len(view.EffectiveGroups))
		for i, group := range view.EffectiveGroups {
			groups[i] = render.Clean(group)
		}
		fmt.Fprintf(&out, "\neffective groups: [%s]\n", strings.Join(groups, ", "))
	}

	if len(view.Associations) > 0 {
		out.WriteString("\nassociations\n------------\n")
		for _, association := range view.Associations {
			fmt.Fprintf(&out, "%s <-> %s\n",
				render.Clean(association.Group1),
				render.Clean(association.Group2),
			)
		}
	}
	_, err = io.WriteString(c.out, out.String())
	return err
}

// topologyTree converts a normalized view into ordered display rows.
func topologyTree(
	view topology.View,
	opts TopologyOptions,
) render.Tree {
	presentation := topologyPresentation{
		options:  opts,
		children: make(map[string][]topology.ViewNode),
	}
	for _, node := range view.Nodes {
		parent := node.DisplayParent
		presentation.children[parent] = append(presentation.children[parent], node)
	}

	tree := render.Tree{
		Headers: []string{"CIDR", "NAME", "GROUPS"},
	}
	for _, root := range presentation.children[""] {
		tree.Roots = append(tree.Roots, presentation.node(root))
	}
	return tree
}

func (p topologyPresentation) node(
	node topology.ViewNode,
) render.Node {
	result := render.Node{
		Cells: p.row(node),
	}
	for _, child := range p.children[node.Cidr.Name] {
		result.Children = append(result.Children, p.node(child))
	}
	return result
}

func (p topologyPresentation) row(
	node topology.ViewNode,
) []render.Cell {
	name := node.Cidr.Name
	if node.PeerName != "" && node.PeerName != name {
		name += " (peer: " + node.PeerName + ")"
	}

	if node.Subject {
		name += " (you)"
	}

	groups := ""
	if len(node.Groups) > 0 {
		groups = "[" + strings.Join(node.Groups, ", ") + "]"
	}

	nameCell := render.Cell{
		Text:      name,
		Indicator: " ",
	}
	switch {
	case node.Subject:
		nameCell.Indicator = "◆"
		nameCell.Color = render.Cyan
	case node.PeerName == "":
		// Reserve the indicator space for alignment.
	case p.options.Connected[node.Cidr.Name]:
		nameCell.Indicator = "◉"
		nameCell.Color = render.Green
	default:
		nameCell.Indicator = "◯"
	}

	cells := []render.Cell{
		{Text: node.Cidr.Cidr},
		nameCell,
		{Text: groups},
	}
	for i := range cells {
		cells[i].Bold = node.Subject && p.options.BoldSubject
	}
	return cells
}

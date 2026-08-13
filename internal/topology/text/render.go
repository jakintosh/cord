package text

import (
	"fmt"
	"io"
	"strings"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// Options controls topology text rendering.
type Options struct {
	Heading     string
	Metadata    string
	BoldSubject bool
}

// Render writes a deterministic topology tree followed by effective groups
// and associations when present.
func Render(
	w io.Writer,
	view topology.View,
	opts Options,
) error {
	view, err := topology.NormalizeView(view)
	if err != nil {
		return err
	}

	heading := opts.Heading
	if heading == "" {
		heading = "topology"
	}
	if _, err := fmt.Fprintln(w, heading); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(heading))); err != nil {
		return err
	}
	if opts.Metadata != "" {
		if _, err := fmt.Fprintln(w, opts.Metadata); err != nil {
			return err
		}
	}

	children := make(map[string][]topology.ViewNode)
	var roots []topology.ViewNode
	for _, node := range view.Nodes {
		if node.DisplayParent == "" {
			roots = append(roots, node)
		} else {
			children[node.DisplayParent] = append(children[node.DisplayParent], node)
		}
	}

	for index, root := range roots {
		last := index == len(roots)-1
		if err := renderNode(w, root, children, "", last, true, opts.BoldSubject); err != nil {
			return err
		}
	}

	if len(view.EffectiveGroups) > 0 {
		if _, err := fmt.Fprintf(
			w,
			"\neffective groups: [%s]\n",
			strings.Join(view.EffectiveGroups, ", "),
		); err != nil {
			return err
		}
	}

	if len(view.Associations) > 0 {
		if _, err := fmt.Fprintln(w, "\nassociations\n------------"); err != nil {
			return err
		}
		for _, association := range view.Associations {
			if _, err := fmt.Fprintf(
				w,
				"%s <-> %s\n",
				association.Group1,
				association.Group2,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderNode(
	w io.Writer,
	node topology.ViewNode,
	children map[string][]topology.ViewNode,
	prefix string,
	last bool,
	root bool,
	boldSubject bool,
) error {
	connector := ""
	nextPrefix := prefix
	if !root {
		if last {
			connector = "└─ "
			nextPrefix += "   "
		} else {
			connector = "├─ "
			nextPrefix += "│  "
		}
	}

	line := fmt.Sprintf("%s => %s", node.Cidr.Cidr, node.Cidr.Name)
	if len(node.Groups) > 0 {
		line += " [" + strings.Join(node.Groups, ", ") + "]"
	}
	if node.Subject {
		line += " (you)"
		if boldSubject {
			line = "\x1b[1m" + line + "\x1b[0m"
		}
	}
	if _, err := fmt.Fprintf(w, "%s%s%s\n", prefix, connector, line); err != nil {
		return err
	}

	childNodes := children[node.Cidr.Name]
	for index, child := range childNodes {
		if err := renderNode(
			w,
			child,
			children,
			nextPrefix,
			index == len(childNodes)-1,
			false,
			boldSubject,
		); err != nil {
			return err
		}
	}
	return nil
}

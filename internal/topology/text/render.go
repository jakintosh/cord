package text

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// Options controls topology text rendering.
type Options struct {
	Heading     string
	Metadata    string
	BoldSubject bool
	Color       bool
	Connected   map[string]bool
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

	rows := make([]renderRow, 0, len(view.Nodes))
	for index, root := range roots {
		last := index == len(roots)-1
		renderNode(&rows, root, children, "", last, true, opts.Connected)
	}

	if err := writeTable(w, rows, opts); err != nil {
		return err
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

type renderRow struct {
	cidr      string
	name      string
	groups    string
	peer      bool
	connected bool
	subject   bool
}

func renderNode(
	rows *[]renderRow,
	node topology.ViewNode,
	children map[string][]topology.ViewNode,
	prefix string,
	last bool,
	root bool,
	connected map[string]bool,
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

	name := node.Cidr.Name
	if node.PeerName != "" && node.PeerName != node.Cidr.Name {
		name += " (peer: " + node.PeerName + ")"
	}
	if node.Subject {
		name += " (you)"
	}
	groupText := ""
	if len(node.Groups) > 0 {
		groupText = "[" + strings.Join(node.Groups, ", ") + "]"
	}
	peer := node.PeerName != ""
	peerConnected := false
	if peer {
		peerConnected = connected[node.Cidr.Name]
	}
	*rows = append(*rows, renderRow{
		cidr:      prefix + connector + node.Cidr.Cidr,
		name:      name,
		groups:    groupText,
		peer:      peer,
		connected: peerConnected,
		subject:   node.Subject,
	})

	childNodes := children[node.Cidr.Name]
	for index, child := range childNodes {
		if err := renderNode(
			rows,
			child,
			children,
			nextPrefix,
			index == len(childNodes)-1,
			false,
			connected,
		); err != nil {
			return err
		}
	}
	return nil
}

func writeTable(
	w io.Writer,
	rows []renderRow,
	opts Options,
) error {
	widths := [3]int{utf8.RuneCountInString("CIDR"), utf8.RuneCountInString("NAME"), utf8.RuneCountInString("GROUPS")}
	for _, row := range rows {
		cells := [3]string{row.cidr, row.name, row.groups}
		for index, cell := range cells {
			if width := utf8.RuneCountInString(cell); width > widths[index] {
				widths[index] = width
			}
		}
	}

	header := [3]string{"CIDR", "NAME", "GROUPS"}
	headerLine := formatCells(header, " ", widths)
	if opts.Color {
		headerLine = "\x1b[1m" + headerLine + "\x1b[0m"
	}
	if _, err := fmt.Fprintln(w, headerLine); err != nil {
		return err
	}

	for _, row := range rows {
		status := " "
		if row.peer {
			status = "◯"
			if row.connected {
				status = "◉"
			}
		}
		line := formatCells([3]string{row.cidr, row.name, row.groups}, status, widths)
		if opts.Color {
			if row.peer && row.connected {
				line = strings.Replace(line, "◉", "\x1b[32m◉\x1b[0m", 1)
			}
		}
		if row.subject && opts.BoldSubject {
			line = "\x1b[1m" + line + "\x1b[0m"
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func formatCells(
	cells [3]string,
	status string,
	widths [3]int,
) string {
	line := padRight(cells[0], widths[0]) + "  " + status + " " + padRight(cells[1], widths[1])
	if cells[2] != "" {
		line += "  " + cells[2]
	}
	return line
}

func padRight(value string, width int) string {
	return value + strings.Repeat(" ", width-utf8.RuneCountInString(value))
}

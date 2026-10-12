package render

import (
	"io"
	"slices"
)

// Node is one table row and its ordered children. Branches prefix the first cell.
type Node struct {
	Cells    []Cell
	Children []Node
}

// Tree is an ordered forest with shared column headers.
type Tree struct {
	Headers []string
	Roots   []Node
}

// Render draws branches in the first column and aligns all rows as a table.
func (t Tree) Render(
	w io.Writer,
	opts Options,
) error {
	table := Table{
		Headers: t.Headers,
	}
	for _, root := range t.Roots {
		root.appendRows(&table, "", "")
	}
	return table.Render(w, opts)
}

func (n Node) appendRows(
	table *Table,
	prefix string,
	connector string,
) {
	cells := slices.Clone(n.Cells)
	if len(cells) == 0 {
		cells = []Cell{{}}
	}
	cells[0].Text = prefix + connector + Clean(cells[0].Text)
	table.Rows = append(table.Rows, cells)
	switch connector {
	case "├─ ":
		prefix += "│  "
	case "└─ ":
		prefix += "   "
	}
	for i, child := range n.Children {
		branch := "├─ "
		if i == len(n.Children)-1 {
			branch = "└─ "
		}
		child.appendRows(table, prefix, branch)
	}
}

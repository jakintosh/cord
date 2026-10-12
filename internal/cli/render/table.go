// Package render draws domain-independent CLI tables and trees.
package render

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Color applies to a cell's indicator, never its text.
type Color string

const (
	Green  Color = "32"
	Yellow Color = "33"
	Red    Color = "31"
	Gray   Color = "90"
	Cyan   Color = "36"
)

// Cell separates text from its optional, independently styled indicator.
type Cell struct {
	Text      string
	Indicator string
	Color     Color
	Bold      bool
}

// Options enables terminal styling. Plain output has identical column widths.
type Options struct {
	Style bool
}

// Cells constructs an unstyled row.
func Cells(
	values ...string,
) []Cell {
	cells := make([]Cell, len(values))
	for i, value := range values {
		cells[i].Text = value
	}
	return cells
}

// Table is a sequence of cells arranged under shared column headers.
type Table struct {
	Headers []string
	Rows    [][]Cell
}

// Render reserves an indicator slot in columns that use one. Rows may omit
// trailing cells, but cannot exceed the header width.
func (t Table) Render(
	w io.Writer,
	opts Options,
) error {
	headers, rows := t.Headers, t.Rows
	widths := make([]int, len(headers))
	indicators := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(Clean(h))
	}
	for _, row := range rows {
		if len(row) > len(headers) {
			return fmt.Errorf("table row has %d cells for %d columns", len(row), len(headers))
		}
		for i, cell := range row {
			indicators[i] = max(indicators[i], utf8.RuneCountInString(Clean(cell.Indicator)))
		}
	}
	for _, row := range rows {
		for i, cell := range row {
			size := utf8.RuneCountInString(Clean(cell.Text))
			if indicators[i] > 0 {
				size += indicators[i] + 1
			}
			widths[i] = max(widths[i], size)
		}
	}
	var out strings.Builder
	layout := tableLayout{
		widths:     widths,
		indicators: indicators,
		style:      opts.Style,
	}

	layout.writeRow(&out, Cells(headers...), true)
	for _, row := range rows {
		layout.writeRow(&out, row, false)
	}
	_, err := io.WriteString(w, out.String())
	return err
}

type tableLayout struct {
	widths     []int
	indicators []int
	style      bool
}

func (layout tableLayout) writeRow(
	out *strings.Builder,
	row []Cell,
	heading bool,
) {
	last := len(row) - 1
	for last >= 0 && row[last].Text == "" && row[last].Indicator == "" {
		last--
	}
	for i, cell := range row[:last+1] {
		if i > 0 {
			out.WriteString("  ")
		}
		text, indicator := Clean(cell.Text), Clean(cell.Indicator)
		bold := layout.style && (heading || cell.Bold)
		if bold {
			out.WriteString("\x1b[1m")
		}
		size := utf8.RuneCountInString(text)
		if !heading && layout.indicators[i] > 0 {
			if layout.style && cell.Color != "" && indicator != "" {
				fmt.Fprintf(out, "\x1b[%sm%s\x1b[39m", cell.Color, indicator)
			} else {
				out.WriteString(indicator)
			}
			out.WriteString(strings.Repeat(" ", layout.indicators[i]-utf8.RuneCountInString(indicator)+1))
			size += layout.indicators[i] + 1
		}
		out.WriteString(text)
		if bold {
			out.WriteString("\x1b[22m")
		}
		if i < last {
			out.WriteString(strings.Repeat(" ", layout.widths[i]-size))
		}
	}
	out.WriteByte('\n')
}

// Clean keeps external text on one line without terminal control characters.
func Clean(
	s string,
) string {
	s = strings.ReplaceAll(s, "\n", "; ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

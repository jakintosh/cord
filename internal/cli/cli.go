// Package cli is Cord's command-line presentation layer. Commands own argument
// parsing and API calls; CLI owns response presentation and output.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
)

// CLI presents API responses to one output destination.
type CLI struct {
	out     io.Writer
	format  Format
	style   bool
	verbose bool
	clock   func() time.Time
}

// Options configures presentation without tying it to process-global output.
type Options struct {
	Format  Format
	Style   bool
	Verbose bool
	Clock   func() time.Time
}

// Format selects how command responses are written. Text is the default.
type Format uint8

const (
	FormatText Format = iota
	FormatJSON
)

// New constructs a presenter. The caller owns the writer's lifetime.
func New(
	out io.Writer,
	opts Options,
) *CLI {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	return &CLI{
		out:     out,
		format:  opts.Format,
		style:   opts.Style,
		verbose: opts.Verbose,
		clock:   clock,
	}
}

// Message writes a human-readable confirmation. Commands with no response
// remain silent in JSON mode; their exit status indicates success.
func (c *CLI) Message(
	message string,
	args ...any,
) error {
	if c.format == FormatJSON {
		return nil
	}

	_, err := fmt.Fprintf(c.out, message, args...)
	return err
}

// Result writes a response as JSON or its human-readable confirmation.
func (c *CLI) Result(
	value any,
	message string,
	args ...any,
) error {
	if c.format == FormatJSON {
		return c.JSON(value)
	}

	return c.Message(message, args...)
}

// Names presents a list of names, retaining an empty JSON array for no results.
func (c *CLI) Names(
	names []string,
) error {
	if c.format == FormatJSON {
		if names == nil {
			names = []string{}
		}
		return c.JSON(names)
	}

	for _, name := range names {
		if _, err := fmt.Fprintln(c.out, render.Clean(name)); err != nil {
			return err
		}
	}
	return nil
}

// JSON writes the complete response using the CLI's existing indented format.
func (c *CLI) JSON(
	value any,
) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to json: %w", err)
	}

	_, err = fmt.Fprintln(c.out, string(data))
	return err
}

// table writes text rows after the presenter has selected the output format.
func (c *CLI) table(
	headers []string,
	rows [][]string,
) error {
	table := render.Table{
		Headers: headers,
	}
	for _, row := range rows {
		table.Rows = append(table.Rows, render.Cells(row...))
	}

	return table.Render(c.out, render.Options{})
}

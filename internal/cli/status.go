package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
)

type statusActivity struct {
	NextAttemptAt   *string
	IntervalSeconds int64
	LastAttemptAt   *string
	LastSuccessAt   *string
	Error           string
}

type statusResource struct {
	kind       string
	name       string
	state      string
	target     string
	health     string
	reason     string
	err        string
	activities map[string]statusActivity
	children   []statusResource
}

type savedNetwork struct {
	name       string
	health     string
	reason     string
	activities map[string]statusActivity
}

type statusInstall struct {
	name  string
	phase string
}

// Saved summaries supply intent-aware health; owned snapshots supply lifecycle
// state. Neither side may hide entries that exist only on the other side.
func (root *statusResource) mergeNetworks(
	networks []savedNetwork,
) {
	for _, saved := range networks {
		var owned *statusResource
		for i := range root.children {
			n := &root.children[i]
			if n.kind == "network" && n.name == saved.name {
				owned = n
				break
			}
		}
		if owned == nil {
			root.children = append(root.children, statusResource{
				kind:   "network",
				name:   saved.name,
				health: saved.health,
				reason: saved.reason,
			})
			continue
		}
		owned.health = saved.health
		if saved.reason != "" {
			owned.reason = saved.reason
		}
		for name, a := range saved.activities {
			if _, ok := owned.activities[name]; !ok {
				owned.activities[name] = a
			}
		}
	}
}

// statusPresentation carries one render's clock and collected diagnostics.
type statusPresentation struct {
	verbose bool
	now     time.Time
	errors  []string
}

func (c *CLI) status(
	root statusResource,
	version string,
	installs []statusInstall,
) error {
	presentation := statusPresentation{
		verbose: c.verbose,
		now:     c.clock(),
	}
	headers := []string{"RESOURCE", "STATUS", "STATE", "LAST SUCCESS"}
	if c.verbose {
		headers = append(headers, "LAST ATTEMPT", "INTERVAL", "NEXT ATTEMPT")
	}
	tree := render.Tree{
		Headers: headers,
		Roots:   []render.Node{root.node(&presentation, "", true)},
	}
	var out strings.Builder
	heading := root.name + " status"
	fmt.Fprintf(&out, "%s\n%s\nversion: %s\n\n",
		heading, strings.Repeat("-", len(heading)), render.Clean(version))
	style := render.Options{
		Style: c.style,
	}
	if err := tree.Render(&out, style); err != nil {
		return err
	}
	if len(presentation.errors) > 0 {
		out.WriteString("\nerrors\n------\n")
		for _, message := range presentation.errors {
			fmt.Fprintln(&out, message)
		}
	}
	if len(installs) > 0 {
		out.WriteString("\ninstalls\n--------\n")
		slices.SortFunc(installs, func(a, b statusInstall) int {
			return strings.Compare(a.name, b.name)
		})
		table := render.Table{
			Headers: []string{"NAME", "PHASE"},
		}
		for _, install := range installs {
			table.Rows = append(table.Rows, render.Cells(install.name, install.phase))
		}
		if err := table.Render(&out, style); err != nil {
			return err
		}
	}
	_, err := io.WriteString(c.out, out.String())
	return err
}

func (r statusResource) node(
	p *statusPresentation,
	parent string,
	root bool,
) render.Node {
	name := r.name
	if name == "" {
		name = r.kind
	}
	label := name
	if !root && r.name != "" {
		label += " (" + r.kind + ")"
	}
	path := name
	if parent != "" {
		path = parent + " / " + name
	}
	result := render.Node{
		Cells: render.Cells(label, "", r.lifecycle(), ""),
	}
	result.Cells[1] = statusCell(r.health)
	p.diagnostic(path, r.reason)
	p.diagnostic(path, r.err)
	for _, name := range r.activityNames(p.verbose) {
		activity := r.activities[name]
		result.Children = append(result.Children, activity.node(name, p))
		p.diagnostic(path+" / "+name, activity.diagnostic(p.now))
	}
	children := r.children
	if root {
		children = slices.Clone(children)
		slices.SortStableFunc(children, func(a, b statusResource) int {
			if a.kind != b.kind {
				return strings.Compare(a.kind, b.kind)
			}
			return strings.Compare(a.name, b.name)
		})
	}
	for _, child := range children {
		result.Children = append(result.Children, child.node(p, path, false))
	}
	return result
}

func (r statusResource) lifecycle() string {
	if r.state == "" {
		return "—"
	}
	if r.target != "" && r.state != r.target {
		return r.state + " → " + r.target
	}
	return r.state
}

func (r statusResource) activityNames(
	verbose bool,
) []string {
	var names []string
	for name, activity := range r.activities {
		// Old operational failures do not affect stopped owners. Cleanup
		// remains actionable; verbose output retains all activity history.
		cleanup := name == "close" || name == "cleanup" || name == "dns-cleanup"
		retired := r.target == "stopped" && !cleanup
		if verbose || (!retired && (defaultActivity(name) || activity.Error != "")) {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		if order := activityOrder(a) - activityOrder(b); order != 0 {
			return order
		}
		return strings.Compare(a, b)
	})
	return names
}

func (p *statusPresentation) diagnostic(
	path string,
	message string,
) {
	if message != "" {
		p.errors = append(p.errors, render.Clean(path)+": "+render.Clean(message))
	}
}

func (a statusActivity) node(
	name string,
	p *statusPresentation,
) render.Node {
	outcome := "not run"
	if a.Error != "" {
		outcome = "error"
	} else if a.LastAttemptAt != nil || a.LastSuccessAt != nil {
		outcome = "ok"
	}
	cells := render.Cells("↻ "+name, "", "", render.Relative(a.LastSuccessAt, p.now, "never"))
	cells[1] = statusCell(outcome)
	if p.verbose {
		interval := "—"
		if a.IntervalSeconds > 0 {
			interval = render.Duration(time.Duration(a.IntervalSeconds) * time.Second)
		}
		cells = append(cells, render.Cells(
			render.Relative(a.LastAttemptAt, p.now, "never"),
			interval,
			render.Relative(a.NextAttemptAt, p.now, "—"),
		)...)
	}
	return render.Node{
		Cells: cells,
	}
}

func (a statusActivity) diagnostic(
	now time.Time,
) string {
	if a.Error == "" || a.LastAttemptAt == nil {
		return a.Error
	}
	return a.Error + " (last attempt " + render.Relative(a.LastAttemptAt, now, "never") + ")"
}

func defaultActivity(
	name string,
) bool {
	switch name {
	case "reconcile", "sync", "scan", "report":
		return true
	default:
		return false
	}
}

func activityOrder(
	name string,
) int {
	switch name {
	case "reconcile":
		return 0
	case "sync":
		return 1
	case "scan":
		return 2
	case "report":
		return 3
	default:
		return 4
	}
}

func statusCell(
	status string,
) render.Cell {
	cell := render.Cell{
		Text:      status,
		Indicator: "◯",
		Color:     render.Gray,
	}
	switch status {
	case "healthy", "ok":
		cell.Indicator, cell.Color = "◉", render.Green
	case "degraded":
		cell.Indicator, cell.Color = "◉", render.Yellow
	case "error":
		cell.Indicator, cell.Color = "◉", render.Red
	case "":
		cell.Text = "unknown"
	}
	return cell
}

func (c *CLI) networkHealth(
	health string,
	reason string,
) error {
	if health != "degraded" {
		return nil
	}

	if reason == "" {
		_, err := fmt.Fprintf(c.out, "health: %s\n", health)
		return err
	}

	_, err := fmt.Fprintf(c.out, "health: %s (%s)\n", render.Clean(health), render.Clean(reason))

	return err
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/command-go/pkg/version"
)

const (
	BIN_NAME = "cord"
	AUTHOR   = "jakintosh"
)

func main() {
	root.Parse()
}

var root = &args.Command{
	Name: BIN_NAME,
	Help: "coordinate wireguard networks",
	Config: &args.Config{
		Author:  AUTHOR,
		Version: VersionInfo.Version,
		HelpOption: &args.HelpOption{
			Short: 'h',
			Long:  "help",
		},
	},
	Subcommands: []*args.Command{
		version.Command(VersionInfo),
		serverCmd,
		clientCmd,
	},
}

func printJSON(
	v any,
) error {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to json: %w", err)
	}
	fmt.Println(string(jsonBytes))
	return nil
}

// printTable prints an aligned table with a header row followed by data
// rows. Each row must have the same number of columns as header.
func printTable(
	header []string,
	rows [][]string,
) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func terminalStyleEnabled(
	file *os.File,
) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func humanizeOptionalTime(timestamp *string) string {
	if timestamp == nil {
		return "never"
	}
	return humanizeSince(*timestamp)
}

// humanizeSince renders an RFC3339 timestamp as a coarse relative time. An
// empty string renders as "never" and malformed input is returned unchanged.
func humanizeSince(
	timestamp string,
) string {
	return humanizeRelativeAt(timestamp, time.Now())
}

func humanizeRelativeAt(
	timestamp string,
	now time.Time,
) string {
	if timestamp == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}

	d := now.Sub(t)
	if d == 0 {
		return "now"
	}
	if d < 0 {
		return "in " + humanizeDuration(-d)
	}
	return humanizeDuration(d) + " ago"
}

func humanizeDuration(
	d time.Duration,
) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client"
	"git.studiopollinator.com/pollinator/cord/internal/server"
)

var jsonOption = args.Option{
	Long: "json",
	Type: args.OptionTypeFlag,
	Help: "emit JSON instead of text",
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

// serverSocket resolves the server daemon socket path from the CLI input,
// falling back to the server package's default.
func serverSocket(
	i *args.Input,
) string {
	return i.GetParameterOr("socket-path", server.DefaultSocketPath)
}

// clientSocket resolves the client daemon socket path from the CLI input,
// falling back to the client package's default.
func clientSocket(
	i *args.Input,
) string {
	return i.GetParameterOr("socket-path", client.DefaultSocketPath)
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

// humanizeSince renders an RFC3339 timestamp (as returned by the API)
// as a coarse "N unit ago" duration for human-mode output. An empty
// string (never handshaked) renders as "never".
func humanizeSince(
	timestamp string,
) string {
	if timestamp == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
)

// jsonOption is shared by every read command; mutations don't take it.
var jsonOption = args.Option{
	Long: "json",
	Type: args.OptionTypeFlag,
	Help: "emit JSON instead of text",
}

func printJSON(v any) error {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to json: %w", err)
	}
	fmt.Println(string(jsonBytes))
	return nil
}

// printTable prints left-aligned columns sized to their widest cell,
// separated by a two-space gutter.
func printTable(header []string, rows [][]string) {
	widths := make([]int, len(header))
	for i, cell := range header {
		widths[i] = len(cell)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printRow := func(row []string) {
		var b strings.Builder
		for i, cell := range row {
			if i == len(row)-1 {
				b.WriteString(cell)
				break
			}
			fmt.Fprintf(&b, "%-*s  ", widths[i], cell)
		}
		fmt.Println(b.String())
	}

	printRow(header)
	for _, row := range rows {
		printRow(row)
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func truncateKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12] + "…"
}

func formatUnix(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return formatTime(time.Unix(ts, 0))
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func inviteState(redeemed bool, expiration time.Time) string {
	switch {
	case redeemed:
		return "redeemed"
	case expiration.Before(time.Now()):
		return "expired"
	default:
		return "active"
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
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

// parseDurationOpt parses a Go duration string from a CLI option value.
// If raw is nil (option not provided), it returns zero and no error.
func parseDurationOpt(
	raw *string,
) (
	time.Duration,
	error,
) {
	if raw == nil {
		return 0, nil
	}
	d, err := time.ParseDuration(*raw)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q: %w", *raw, err)
	}
	return d, nil
}

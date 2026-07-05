package main

import (
	"encoding/json"
	"fmt"

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

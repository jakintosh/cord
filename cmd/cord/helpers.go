package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client"
	clientapi "git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	serverapi "git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

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

// serverClient resolves the socket path and returns an admin API client
// for the server daemon.
func serverClient(
	i *args.Input,
) (
	*serverapi.Client,
	error,
) {
	return serverapi.NewClient(serverSocket(i))
}

// clientClient resolves the socket path and returns an API client for the
// client daemon.
func clientClient(
	i *args.Input,
) (
	*clientapi.Client,
	error,
) {
	return clientapi.NewClient(clientSocket(i))
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

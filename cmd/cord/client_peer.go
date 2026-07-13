package main

import (
	"fmt"
	"strconv"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
)

var clientPeerCmd = &args.Command{
	Name: "peer",
	Help: "inspect network peers",
	Subcommands: []*args.Command{
		clientPeerList,
	},
}

var clientPeerList = &args.Command{
	Name: "list",
	Help: "list peers on a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		peers, err := client.ListPeers(i.Context(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(peers)
		}

		printClientPeers(peers)
		return nil
	},
}

// printClientPeers prints a one-row-per-peer summary table.
func printClientPeers(
	peers []api.Peer,
) {
	rows := make([][]string, len(peers))
	for idx, p := range peers {
		lastHandshake := ""
		if p.LastHandshake != nil {
			lastHandshake = *p.LastHandshake
		}
		rows[idx] = []string{
			p.Name,
			p.Route,
			p.Endpoint,
			humanizeSince(lastHandshake),
			strconv.FormatBool(p.Connected),
		}
	}
	printTable([]string{"NAME", "ROUTE", "ENDPOINT", "LAST HANDSHAKE", "CONNECTED"}, rows)
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

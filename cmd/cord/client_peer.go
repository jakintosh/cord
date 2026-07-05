package main

import (
	"context"
	"strconv"

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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		peers, err := client.ListPeers(context.Background(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(peers)
		}
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
		return nil
	},
}

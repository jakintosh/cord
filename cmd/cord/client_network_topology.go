package main

import (
	"fmt"
	"os"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	topotext "git.studiopollinator.com/pollinator/cord/internal/topology/text"
)

var clientNetworkTopology = &args.Command{
	Name: "topology",
	Help: "show this client's projected network topology",
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

		result, err := client.GetNetworkTopology(i.Context(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}

		view, err := result.ToView()
		if err != nil {
			return err
		}

		syncTime := result.SyncedAt.Format(time.RFC3339)
		connected := make(map[string]bool, len(result.Nodes))
		for _, node := range result.Nodes {
			if node.Connected != nil {
				connected[node.Name] = *node.Connected
			}
		}
		styled := terminalStyleEnabled(os.Stdout)
		return topotext.Render(os.Stdout, view, topotext.Options{
			Heading:     "topology (projected)",
			Metadata:    fmt.Sprintf("synced: %s", humanizeSince(syncTime)),
			BoldSubject: styled,
			Color:       styled,
			Connected:   connected,
		})
	},
}

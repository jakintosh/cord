package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
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

		return cliApp(i).ClientPeers(peers)
	},
}

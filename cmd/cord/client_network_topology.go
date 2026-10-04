package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
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

		return cliApp(i).ClientTopology(result)
	},
}

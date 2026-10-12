package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
)

var serverNetworkTopology = &args.Command{
	Name: "topology",
	Help: "show a network's full CIDR and group topology",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		result, err := client.GetNetworkTopology(i.Context(), network)
		if err != nil {
			return err
		}

		return cliApp(i).ServerTopology(result)
	},
}

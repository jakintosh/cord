package main

import (
	"context"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
)

var serverAssociationCmd = &args.Command{
	Name: "association",
	Help: "manage CIDR associations",
	Subcommands: []*args.Command{
		serverAssociationAdd,
		serverAssociationDelete,
		serverAssociationList,
	},
}

var serverAssociationAdd = &args.Command{
	Name: "add",
	Help: "associate two CIDRs",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "cidr1",
			Help: "first CIDR name",
		},
		{
			Name: "cidr2",
			Help: "second CIDR name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		client := api.NewClient(socketPath)
		result, err := client.AddAssociation(context.Background(), network, api.AddAssociationRequest{
			Cidr1: cidr1,
			Cidr2: cidr2,
		})
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var serverAssociationDelete = &args.Command{
	Name: "delete",
	Help: "remove a CIDR association",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "cidr1",
			Help: "first CIDR name",
		},
		{
			Name: "cidr2",
			Help: "second CIDR name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		client := api.NewClient(socketPath)
		if err := client.DeleteAssociation(context.Background(), network, api.DeleteAssociationRequest{
			Cidr1: cidr1,
			Cidr2: cidr2,
		}); err != nil {
			return err
		}

		return printJSON(api.DeleteResponse{
			Status: "deleted",
			ID:     cidr1 + "/" + cidr2,
		})
	},
}

var serverAssociationList = &args.Command{
	Name: "list",
	Help: "list CIDR associations on a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		associations, err := client.ListAssociations(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(associations)
	},
}

package main

import (
	"context"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
)

var serverCidrCmd = &args.Command{
	Name: "cidr",
	Help: "manage CIDRs",
	Subcommands: []*args.Command{
		serverCidrAdd,
		serverCidrRename,
		serverCidrDelete,
		serverCidrList,
	},
}

var serverCidrAdd = &args.Command{
	Name: "add",
	Help: "add a CIDR to a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "name",
			Help: "CIDR name",
		},
		{
			Name: "cidr",
			Help: "address range in CIDR notation",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")

		client := api.NewClient(socketPath)
		result, err := client.AddCidr(context.Background(), network, api.AddCidrRequest{
			Name: name,
			Cidr: cidr,
		})
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var serverCidrRename = &args.Command{
	Name: "rename",
	Help: "rename a CIDR",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "cidr",
			Help: "current CIDR name",
		},
		{
			Name: "new-name",
			Help: "new CIDR name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		newName := i.GetOperand("new-name")

		client := api.NewClient(socketPath)
		result, err := client.RenameCidr(context.Background(), network, cidr, newName)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var serverCidrDelete = &args.Command{
	Name: "delete",
	Help: "delete a CIDR",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "cidr",
			Help: "CIDR name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		client := api.NewClient(socketPath)
		if err := client.DeleteCidr(context.Background(), network, cidr); err != nil {
			return err
		}

		return printJSON(api.DeleteResponse{
			Status: "deleted",
			ID:     cidr,
		})
	},
}

var serverCidrList = &args.Command{
	Name: "list",
	Help: "list CIDRs on a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		cidrs, err := client.ListCidrs(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(cidrs)
	},
}

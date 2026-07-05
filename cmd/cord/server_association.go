package main

import (
	"context"
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
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
		socketPath := serverSocket(i)
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		client := admin.NewClient(socketPath)
		result, err := client.AddAssociation(context.Background(), network, admin.AddAssociationRequest{
			Cidr1: cidr1,
			Cidr2: cidr2,
		})
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}
		fmt.Printf("cidrs %q and %q associated\n", cidr1, cidr2)
		return nil
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
		socketPath := serverSocket(i)
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		client := admin.NewClient(socketPath)
		result, err := client.DeleteAssociation(context.Background(), network, admin.DeleteAssociationRequest{
			Cidr1: cidr1,
			Cidr2: cidr2,
		})
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}
		fmt.Printf("association %q deleted\n", cidr1+"/"+cidr2)
		return nil
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
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		network := i.GetOperand("network")

		client := admin.NewClient(socketPath)
		associations, err := client.ListAssociations(context.Background(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(associations)
		}
		rows := make([][]string, len(associations))
		for idx, a := range associations {
			rows[idx] = []string{a.Cidr1, a.Cidr2}
		}
		printTable([]string{"CIDR1", "CIDR2"}, rows)
		return nil
	},
}

package main

import (
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
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.AddAssociation(
			i.Context(),
			network,
			cidr1,
			cidr2,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
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
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.DeleteAssociation(
			i.Context(),
			network,
			cidr1,
			cidr2,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
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
		network := i.GetOperand("network")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		associations, err := client.ListAssociations(
			i.Context(),
			network,
		)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(associations)
		}

		printAssociations(associations)
		return nil
	},
}

// printAssociations prints a one-row-per-association summary table.
func printAssociations(
	associations []admin.Association,
) {
	rows := make([][]string, len(associations))
	for idx, a := range associations {
		rows[idx] = []string{a.Cidr1, a.Cidr2}
	}
	printTable([]string{"CIDR1", "CIDR2"}, rows)
}

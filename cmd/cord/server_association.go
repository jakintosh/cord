package main

import (
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
)

var serverAssociationCmd = &args.Command{
	Name: "association",
	Help: "manage associations between a network's CIDRs",
	Subcommands: []*args.Command{
		serverAssociationAdd,
		serverAssociationDelete,
		serverAssociationList,
	},
}

var serverAssociationAdd = &args.Command{
	Name: "add",
	Help: "create an association between two CIDRs",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to add an association to",
		},
		{
			Name: "cidr1",
			Help: "name of the first CIDR",
		},
		{
			Name: "cidr2",
			Help: "name of the second CIDR",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.CreateAssociation(cidr1, cidr2)
		if err != nil {
			return fmt.Errorf("failed to create association: %w", err)
		}

		return nil
	},
}

var serverAssociationDelete = &args.Command{
	Name: "delete",
	Help: "delete an association between two CIDRs",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to delete an association from",
		},
		{
			Name: "cidr1",
			Help: "name of the first CIDR",
		},
		{
			Name: "cidr2",
			Help: "name of the second CIDR",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.DeleteAssociation(cidr1, cidr2)
		if err != nil {
			return fmt.Errorf("failed to delete association: %w", err)
		}

		return nil
	},
}

var serverAssociationList = &args.Command{
	Name: "list",
	Help: "list a network's CIDR associations",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to list associations from",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return err
		}

		// execute
		associations, err := srv.ListAssociations()
		if err != nil {
			return fmt.Errorf("failed to list associations: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(associations)
		}
		rows := make([][]string, 0, len(associations))
		for _, association := range associations {
			rows = append(rows, []string{
				association.Cidr1,
				association.Cidr2,
			})
		}
		printTable([]string{"CIDR1", "CIDR2"}, rows)
		return nil
	},
}

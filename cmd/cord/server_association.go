package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
)

var serverAssociationCmd = &args.Command{
	Name: "association",
	Help: "manage group associations",
	Subcommands: []*args.Command{
		serverAssociationCreate,
		serverAssociationDelete,
		serverAssociationList,
	},
}

var serverAssociationCreate = &args.Command{
	Name: "create",
	Help: "associate two groups",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "group1",
			Help: "first group name",
		},
		{
			Name: "group2",
			Help: "second group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		group1 := i.GetOperand("group1")
		group2 := i.GetOperand("group2")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.AddAssociation(
			i.Context(),
			network,
			group1,
			group2,
		); err != nil {
			return err
		}

		return cliApp(i).Message("groups %q and %q associated\n", group1, group2)
	},
}

var serverAssociationDelete = &args.Command{
	Name: "delete",
	Help: "remove a group association",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "group1",
			Help: "first group name",
		},
		{
			Name: "group2",
			Help: "second group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		group1 := i.GetOperand("group1")
		group2 := i.GetOperand("group2")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.DeleteAssociation(
			i.Context(),
			network,
			group1,
			group2,
		); err != nil {
			return err
		}

		return cliApp(i).Message("association %q deleted\n", group1+"/"+group2)
	},
}

var serverAssociationList = &args.Command{
	Name: "list",
	Help: "list group associations on a network",
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

		return cliApp(i).Associations(associations)
	},
}

package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
)

var serverGroupCmd = &args.Command{
	Name: "group",
	Help: "manage groups",
	Subcommands: []*args.Command{
		serverGroupList,
		serverGroupCreate,
		serverGroupDelete,
	},
}

var serverGroupList = &args.Command{
	Name: "list",
	Help: "list groups on a network",
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

		groups, err := client.ListGroups(
			i.Context(),
			network,
		)
		if err != nil {
			return err
		}

		return cliApp(i).Groups(groups)
	},
}

var serverGroupCreate = &args.Command{
	Name: "create",
	Help: "create a group",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "name",
			Help: "group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.CreateGroup(
			i.Context(),
			network,
			name,
		); err != nil {
			return err
		}

		return cliApp(i).Message("group %q created\n", name)
	},
}

var serverGroupDelete = &args.Command{
	Name: "delete",
	Help: "delete a group",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "name",
			Help: "group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.DeleteGroup(
			i.Context(),
			network,
			name,
		); err != nil {
			return err
		}

		return cliApp(i).Message("group %q deleted\n", name)
	},
}

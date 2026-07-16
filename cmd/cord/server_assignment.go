package main

import (
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

var serverAssignmentCmd = &args.Command{
	Name: "assignment",
	Help: "manage group assignments to CIDRs",
	Subcommands: []*args.Command{
		serverAssignmentAdd,
		serverAssignmentRemove,
		serverAssignmentList,
	},
}

var serverAssignmentAdd = &args.Command{
	Name: "add",
	Help: "assign a group to a CIDR",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "cidr",
			Help: "CIDR name",
		},
		{
			Name: "group",
			Help: "group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		group := i.GetOperand("group")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.AssignGroup(
			i.Context(),
			network,
			cidr,
			group,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("group %q assigned to CIDR %q\n", group, cidr)
		return nil
	},
}

var serverAssignmentRemove = &args.Command{
	Name: "remove",
	Help: "remove a group assignment from a CIDR",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "cidr",
			Help: "CIDR name",
		},
		{
			Name: "group",
			Help: "group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		group := i.GetOperand("group")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.RemoveGroup(
			i.Context(),
			network,
			cidr,
			group,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("group %q removed from CIDR %q\n", group, cidr)
		return nil
	},
}

var serverAssignmentList = &args.Command{
	Name: "list",
	Help: "list group assignments on a network",
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

		assignments, err := client.ListAssignments(
			i.Context(),
			network,
		)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(assignments)
		}

		printAssignments(assignments)
		return nil
	},
}

func printAssignments(
	assignments []admin.Assignment,
) {
	rows := make([][]string, len(assignments))
	for idx, a := range assignments {
		rows[idx] = []string{a.CidrName, a.GroupName}
	}
	printTable([]string{"CIDR", "GROUP"}, rows)
}

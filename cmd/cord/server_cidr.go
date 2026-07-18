package main

import (
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

var serverCidrCmd = &args.Command{
	Name: "cidr",
	Help: "manage CIDRs",
	Subcommands: []*args.Command{
		serverCidrAdd,
		serverCidrRename,
		serverCidrDelete,
		serverCidrList,
		serverCidrGroup,
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
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.AddCidr(
			i.Context(),
			network,
			name,
			cidr,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("cidr %q added\n", name)
		return nil
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
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		newName := i.GetOperand("new-name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.RenameCidr(
			i.Context(),
			network,
			cidr,
			newName,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("cidr %q renamed to %q\n", cidr, newName)
		return nil
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
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.DeleteCidr(
			i.Context(),
			network,
			cidr,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("cidr %q deleted\n", cidr)
		return nil
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
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		cidrs, err := client.ListCidrs(i.Context(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(cidrs)
		}

		printCidrs(cidrs)
		return nil
	},
}

var serverCidrGroup = &args.Command{
	Name: "group",
	Help: "manage groups assigned to a CIDR",
	Subcommands: []*args.Command{
		serverCidrGroupAdd,
		serverCidrGroupRemove,
		serverCidrGroupList,
	},
}

var serverCidrGroupAdd = &args.Command{
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
		if err := client.AssignCidrGroup(
			i.Context(),
			network,
			cidr,
			group,
		); err != nil {
			return err
		}
		if !i.GetFlag("json") {
			fmt.Printf("group %q assigned to CIDR %q\n", group, cidr)
		}
		return nil
	},
}

var serverCidrGroupRemove = &args.Command{
	Name: "remove",
	Help: "remove a group from a CIDR",
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
		if err := client.RemoveCidrGroup(
			i.Context(),
			network,
			cidr,
			group,
		); err != nil {
			return err
		}
		if !i.GetFlag("json") {
			fmt.Printf("group %q removed from CIDR %q\n", group, cidr)
		}
		return nil
	},
}

var serverCidrGroupList = &args.Command{
	Name: "list",
	Help: "list groups assigned to a CIDR",
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
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		client, err := serverClient(i)
		if err != nil {
			return err
		}
		groups, err := client.ListCidrGroups(
			i.Context(),
			network,
			cidr,
		)
		if err != nil {
			return err
		}
		if i.GetFlag("json") {
			return printJSON(groups)
		}
		printGroups(groups)
		return nil
	},
}

// printCidrs prints a one-row-per-CIDR summary table.
func printCidrs(
	cidrs []admin.Cidr,
) {
	rows := make([][]string, len(cidrs))
	for idx, c := range cidrs {
		rows[idx] = []string{c.Name, c.Cidr}
	}
	printTable([]string{"NAME", "CIDR"}, rows)
}

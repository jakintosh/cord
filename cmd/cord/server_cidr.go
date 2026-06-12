package main

import (
	"fmt"
	"strings"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

var serverCidrCmd = &args.Command{
	Name: "cidr",
	Help: "manage a network's CIDRs",
	Subcommands: []*args.Command{
		serverCidrAdd,
		serverCidrRename,
		serverCidrDelete,
		serverCidrList,
	},
}

var serverCidrAdd = &args.Command{
	Name: "add",
	Help: "add a child CIDR to a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to add a CIDR to",
		},
		{
			Name: "name",
			Help: "name of the CIDR",
		},
		{
			Name: "cidr",
			Help: "address range in CIDR notation (i.e. 10.0.0.0/8)",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		// execute command
		req := server.CreateCidrRequest{
			Name: name,
			Cidr: cidr,
		}
		err = srv.CreateCidr(req)
		if err != nil {
			return fmt.Errorf("failed to create cidr: %w", err)
		}

		return nil
	},
}

var serverCidrRename = &args.Command{
	Name: "rename",
	Help: "rename an existing CIDR from a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "cidr",
			Help: "CIDR to rename",
		},
		{
			Name: "new-name",
			Help: "new name for CIDR",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		newName := i.GetOperand("new-name")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		// execute command
		req := server.UpdateCidrRequest{
			Name: newName,
		}
		err = srv.UpdateCidr(cidr, req)
		if err != nil {
			return fmt.Errorf("failed to rename cidr: %w", err)
		}

		return nil
	},
}

var serverCidrDelete = &args.Command{
	Name: "delete",
	Help: "delete an existing CIDR from a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "cidr",
			Help: "CIDR to delete",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.DeleteCidr(cidr)
		if err != nil {
			return fmt.Errorf("failed to delete cidr: %w", err)
		}

		return nil
	},
}

var serverCidrList = &args.Command{
	Name: "list",
	Help: "list a network's CIDRs and their associations",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to list CIDRs from",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create server
		srv, err := newServerRead(i, network)
		if err != nil {
			return err
		}

		// execute
		details, err := srv.ListCidrDetails()
		if err != nil {
			return fmt.Errorf("failed to list cidrs: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(details)
		}
		rows := make([][]string, 0, len(details))
		for _, detail := range details {
			associations := "-"
			if len(detail.Associations) > 0 {
				associations = strings.Join(detail.Associations, ", ")
			}
			rows = append(rows, []string{
				detail.Name,
				detail.Cidr,
				associations,
			})
		}
		printTable([]string{"NAME", "CIDR", "ASSOCIATIONS"}, rows)
		return nil
	},
}

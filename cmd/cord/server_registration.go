package main

import (
	"fmt"
	"os"
	"strconv"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

var serverRegistrationCmd = &args.Command{
	Name: "registration",
	Help: "manage peer registrations",
	Subcommands: []*args.Command{
		serverRegistrationCreate,
		serverRegistrationList,
		serverRegistrationRevoke,
		serverRegistrationGroup,
	},
}

var serverRegistrationCreate = &args.Command{
	Name: "create",
	Help: "create a peer registration on a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "name",
			Help: "peer name",
		},
		{
			Name: "ip",
			Help: "peer IP address",
		},
	},
	Options: []args.Option{
		{
			Short: 'a',
			Long:  "admin",
			Type:  args.OptionTypeFlag,
			Help:  "make the new peer an admin",
		},
		{
			Short: 'o',
			Long:  "output",
			Type:  args.OptionTypeParameter,
			Help:  "write the invitation JSON to this file instead of stdout",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		ip := i.GetOperand("ip")

		admin := i.GetFlag("admin")
		output := i.GetParameter("output")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		invitation, err := client.CreateInvite(i.Context(), network, name, ip, admin)
		if err != nil {
			return err
		}

		if output != nil {
			f, err := os.Create(*output)
			if err != nil {
				return fmt.Errorf("create invitation file: %w", err)
			}
			defer f.Close()

			if err := invitation.Write(f); err != nil {
				return fmt.Errorf("write invitation file: %w", err)
			}

			fmt.Printf("registration %q created; invitation written to %s\n", name, *output)
			return nil
		}

		// The invitation payload is the deliverable, so it is printed
		// as JSON in both human and --json modes unless redirected to
		// a file via --output.
		return printJSON(invitation)
	},
}

var serverRegistrationList = &args.Command{
	Name: "list",
	Help: "list registrations on a network",
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

		registrations, err := client.ListRegistrations(i.Context(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(registrations)
		}

		printRegistrations(registrations)
		return nil
	},
}

var serverRegistrationRevoke = &args.Command{
	Name: "revoke",
	Help: "revoke a peer registration",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "name",
			Help: "registration name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.RevokeRegistration(
			i.Context(),
			network,
			name,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("registration %q revoked\n", name)
		return nil
	},
}

var serverRegistrationGroup = &args.Command{
	Name: "group",
	Help: "manage group assignments for a registration",
	Subcommands: []*args.Command{
		serverRegistrationGroupAdd,
		serverRegistrationGroupRemove,
		serverRegistrationGroupList,
	},
}

var serverRegistrationGroupAdd = &args.Command{
	Name: "add",
	Help: "assign a group to a registration",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "registration",
			Help: "registration name",
		},
		{
			Name: "group",
			Help: "group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		registration := i.GetOperand("registration")
		group := i.GetOperand("group")

		client, err := serverClient(i)
		if err != nil {
			return err
		}
		if err := client.AssignRegistrationGroup(
			i.Context(),
			network,
			registration,
			group,
		); err != nil {
			return err
		}
		if !i.GetFlag("json") {
			fmt.Printf("group %q assigned to registration %q\n", group, registration)
		}
		return nil
	},
}

var serverRegistrationGroupRemove = &args.Command{
	Name: "remove",
	Help: "remove a group from a registration",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "registration",
			Help: "registration name",
		},
		{
			Name: "group",
			Help: "group name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		registration := i.GetOperand("registration")
		group := i.GetOperand("group")

		client, err := serverClient(i)
		if err != nil {
			return err
		}
		if err := client.RemoveRegistrationGroup(
			i.Context(),
			network,
			registration,
			group,
		); err != nil {
			return err
		}
		if !i.GetFlag("json") {
			fmt.Printf("group %q removed from registration %q\n", group, registration)
		}
		return nil
	},
}

var serverRegistrationGroupList = &args.Command{
	Name: "list",
	Help: "list groups assigned to a registration",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "registration",
			Help: "registration name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		registration := i.GetOperand("registration")

		client, err := serverClient(i)
		if err != nil {
			return err
		}
		groups, err := client.ListRegistrationGroups(
			i.Context(),
			network,
			registration,
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

// printRegistrations prints a one-row-per-registration summary table.
func printRegistrations(
	registrations []admin.Registration,
) {
	rows := make([][]string, len(registrations))
	for idx, reg := range registrations {
		rows[idx] = []string{
			reg.Name,
			reg.Route,
			strconv.FormatBool(reg.Admin),
			strconv.FormatBool(reg.Redeemed),
			reg.ExpiresAt,
		}
	}
	printTable([]string{"NAME", "ROUTE", "ADMIN", "REDEEMED", "EXPIRES AT"}, rows)
}

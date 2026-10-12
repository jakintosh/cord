package main

import (
	"fmt"
	"os"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
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

			return cliApp(i).Message("registration %q created; invitation written to %s\n", name, *output)
		}

		// The invitation payload is the deliverable, so it is printed
		// as JSON in both human and --json modes unless redirected to
		// a file via --output.
		return cliApp(i).JSON(invitation)
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

		return cliApp(i).Registrations(registrations)
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

		return cliApp(i).Message("registration %q revoked\n", name)
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
		return cliApp(i).Message("group %q assigned to registration %q\n", group, registration)
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
		return cliApp(i).Message("group %q removed from registration %q\n", group, registration)
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
		return cliApp(i).Groups(groups)
	},
}

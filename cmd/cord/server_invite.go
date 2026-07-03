package main

import (
	"context"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

var serverInviteCmd = &args.Command{
	Name: "invite",
	Help: "manage invites",
	Subcommands: []*args.Command{
		serverInviteCreate,
		serverInviteList,
	},
}

var serverInviteCreate = &args.Command{
	Name: "create",
	Help: "create a peer invite on a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "name",
			Help: "peer name",
		},
	},
	Options: []args.Option{
		{
			Long: "ip",
			Type: args.OptionTypeParameter,
			Help: "peer IP address",
		},
		{
			Short: 'a',
			Long:  "admin",
			Type:  args.OptionTypeFlag,
			Help:  "make the new peer an admin",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)

		network := i.GetOperand("network")
		name := i.GetOperand("name")

		ip := i.GetParameter("ip")
		adminFlag := i.GetFlag("admin")

		client := admin.NewClient(socketPath)
		inv, err := client.CreateInvite(
			context.Background(),
			network,
			admin.CreateInviteRequest{
				Name:  name,
				Ip:    ip,
				Admin: adminFlag,
			},
		)
		if err != nil {
			return err
		}

		return printJSON(inv)
	},
}

var serverInviteList = &args.Command{
	Name: "list",
	Help: "list invites on a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")

		client := admin.NewClient(socketPath)
		invites, err := client.ListRegistrations(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(invites)
	},
}

package main

import (
	"context"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
)

var serverInviteCmd = &args.Command{
	Name: "invite",
	Help: "manage invites",
	Subcommands: []*args.Command{
		serverInviteList,
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

		client := api.NewClient(socketPath)
		invites, err := client.ListInvites(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(invites)
	},
}

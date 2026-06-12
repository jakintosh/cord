package main

import (
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

var serverInviteCmd = &args.Command{
	Name: "invite",
	Help: "inspect a network's peer invites",
	Subcommands: []*args.Command{
		serverInviteList,
	},
}

var serverInviteList = &args.Command{
	Name: "list",
	Help: "list a network's invites and their state",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to list invites from",
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
		invites, err := srv.ListInvites()
		if err != nil {
			return fmt.Errorf("failed to list invites: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(invites)
		}
		printInviteTable(invites)
		return nil
	},
}

func printInviteTable(invites []server.InviteStatus) {
	rows := make([][]string, 0, len(invites))
	for _, invite := range invites {
		rows = append(rows, []string{
			invite.Name,
			invite.NetworkCidr,
			yesNo(invite.Admin),
			inviteState(invite.Redeemed, invite.Expiration),
			formatTime(invite.Expiration),
		})
	}
	printTable([]string{"NAME", "CIDR", "ADMIN", "STATE", "EXPIRES"}, rows)
}

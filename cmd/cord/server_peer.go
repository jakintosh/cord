package main

import (
	"fmt"
	"strconv"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

var serverPeerCmd = &args.Command{
	Name: "peer",
	Help: "manage peers",
	Subcommands: []*args.Command{
		serverPeerRename,
		serverPeerEnable,
		serverPeerDisable,
		serverPeerDelete,
		serverPeerList,
	},
}

var serverPeerRename = &args.Command{
	Name: "rename",
	Help: "rename a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "peer",
			Help: "current peer name",
		},
		{
			Name: "new-name",
			Help: "new peer name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")
		newName := i.GetOperand("new-name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		updated, err := client.UpdatePeer(
			i.Context(),
			network,
			peer,
			&newName,
			nil,
		)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(updated)
		}

		fmt.Printf("peer %q renamed to %q\n", peer, newName)
		return nil
	},
}

var serverPeerEnable = &args.Command{
	Name: "enable",
	Help: "enable a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "peer",
			Help: "peer name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		enabled := true
		if _, err := client.UpdatePeer(
			i.Context(),
			network,
			peer,
			nil,
			&enabled,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("peer %q enabled\n", peer)
		return nil
	},
}

var serverPeerDisable = &args.Command{
	Name: "disable",
	Help: "disable a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "peer",
			Help: "peer name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		disabled := false
		if _, err := client.UpdatePeer(
			i.Context(),
			network,
			peer,
			nil,
			&disabled,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("peer %q disabled\n", peer)
		return nil
	},
}

var serverPeerDelete = &args.Command{
	Name: "delete",
	Help: "delete a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "peer",
			Help: "peer name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.DeletePeer(
			i.Context(),
			network,
			peer,
		); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}

		fmt.Printf("peer %q deleted\n", peer)
		return nil
	},
}

var serverPeerList = &args.Command{
	Name: "list",
	Help: "list peers on a network",
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

		peers, err := client.ListPeers(i.Context(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(peers)
		}

		printServerPeers(peers)
		return nil
	},
}

// printServerPeers prints a one-row-per-peer summary table.
func printServerPeers(
	peers []admin.Peer,
) {
	rows := make([][]string, len(peers))
	for idx, p := range peers {
		rows[idx] = []string{
			p.Name,
			p.Route,
			strconv.FormatBool(p.Admin),
			strconv.FormatBool(p.Enabled),
			p.PublicKey,
		}
	}
	printTable([]string{"NAME", "ROUTE", "ADMIN", "ENABLED", "PUBLIC KEY"}, rows)
}

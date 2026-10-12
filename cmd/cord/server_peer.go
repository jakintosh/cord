package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
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

		return cliApp(i).Result(updated, "peer %q renamed to %q\n", peer, newName)
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

		return cliApp(i).Message("peer %q enabled\n", peer)
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

		return cliApp(i).Message("peer %q disabled\n", peer)
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

		return cliApp(i).Message("peer %q deleted\n", peer)
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

		return cliApp(i).ServerPeers(peers)
	},
}

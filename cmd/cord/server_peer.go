package main

import (
	"context"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/serverd"
)

var serverPeerCmd = &args.Command{
	Name: "peer",
	Help: "manage peers",
	Subcommands: []*args.Command{
		serverPeerAdd,
		serverPeerRename,
		serverPeerEnable,
		serverPeerDisable,
		serverPeerDelete,
		serverPeerList,
		serverPeerVisible,
	},
}

var serverPeerAdd = &args.Command{
	Name: "add",
	Help: "add a peer to a network",
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
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		ip := i.GetOperand("ip")
		admin := i.GetFlag("admin")

		client := serverd.NewClient(socketPath)
		peer, err := client.AddPeer(context.Background(), network, serverd.AddPeerRequest{
			Name:  name,
			Ip:    ip,
			Admin: admin,
		})
		if err != nil {
			return err
		}

		return printJSON(peer)
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
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")
		newName := i.GetOperand("new-name")

		client := serverd.NewClient(socketPath)
		result, err := client.RenamePeer(context.Background(), network, peer, newName)
		if err != nil {
			return err
		}

		return printJSON(result)
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
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client := serverd.NewClient(socketPath)
		result, err := client.EnablePeer(context.Background(), network, peer)
		if err != nil {
			return err
		}

		return printJSON(result)
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
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client := serverd.NewClient(socketPath)
		result, err := client.DisablePeer(context.Background(), network, peer)
		if err != nil {
			return err
		}

		return printJSON(result)
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
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client := serverd.NewClient(socketPath)
		if err := client.DeletePeer(context.Background(), network, peer); err != nil {
			return err
		}

		return printJSON(serverd.DeleteResponse{
			Status: "deleted",
			ID:     peer,
		})
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
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")

		client := serverd.NewClient(socketPath)
		peers, err := client.ListPeers(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(peers)
	},
}

var serverPeerVisible = &args.Command{
	Name: "visible",
	Help: "list peers visible to a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)
		network := i.GetOperand("network")

		client := serverd.NewClient(socketPath)
		peers, err := client.ListPeersVisible(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(peers)
	},
}

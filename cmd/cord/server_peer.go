package main

import (
	"context"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")
		newName := i.GetOperand("new-name")

		client := admin.NewClient(socketPath)
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client := admin.NewClient(socketPath)
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client := admin.NewClient(socketPath)
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		client := admin.NewClient(socketPath)
		if err := client.DeletePeer(context.Background(), network, peer); err != nil {
			return err
		}

		return printJSON(api.DeleteResponse{
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		network := i.GetOperand("network")

		client := admin.NewClient(socketPath)
		peers, err := client.ListPeers(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(peers)
	},
}

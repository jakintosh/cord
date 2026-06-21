package main

import (
	"context"
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
)

var clientNetworkCmd = &args.Command{
	Name: "network",
	Help: "manage client networks",
	Subcommands: []*args.Command{
		clientNetworkList,
		clientNetworkShow,
		clientNetworkInstall,
		clientNetworkUninstall,
		clientNetworkEnable,
		clientNetworkDisable,
		clientNetworkFetch,
	},
}

var clientNetworkList = &args.Command{
	Name:    "list",
	Help:    "list installed networks",
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)

		client := api.NewClient(socketPath)
		networks, err := client.ListNetworks(context.Background())
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(networks)
		}
		for _, n := range networks {
			fmt.Println(n.Name)
		}
		return nil
	},
}

var clientNetworkShow = &args.Command{
	Name: "show",
	Help: "show network details",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.ShowNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var clientNetworkInstall = &args.Command{
	Name: "install",
	Help: "install a network from an invite",
	Operands: []args.Operand{
		{
			Name: "invite",
			Help: "path to the invite file",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		invite := i.GetOperand("invite")

		client := api.NewClient(socketPath)
		result, err := client.InstallNetwork(context.Background(), api.InstallNetworkRequest{
			InvitePath: invite,
		})
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var clientNetworkUninstall = &args.Command{
	Name: "uninstall",
	Help: "uninstall a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		if err := client.UninstallNetwork(context.Background(), network); err != nil {
			return err
		}

		return printJSON(api.DeleteResponse{
			Status: "deleted",
			ID:     network,
		})
	},
}

var clientNetworkEnable = &args.Command{
	Name: "enable",
	Help: "enable a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.EnableNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var clientNetworkDisable = &args.Command{
	Name: "disable",
	Help: "disable a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.DisableNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var clientNetworkFetch = &args.Command{
	Name: "fetch",
	Help: "sync peer state from server",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", api.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.FetchNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

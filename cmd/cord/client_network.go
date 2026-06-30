package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	server "git.studiopollinator.com/pollinator/cord/internal/server/service"
)

var clientNetworkCmd = &args.Command{
	Name: "network",
	Help: "manage client networks",
	Subcommands: []*args.Command{
		clientNetworkList,
		clientNetworkShow,
		clientNetworkInstall,
		clientNetworkRedeem,
		clientNetworkConfirm,
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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)

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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.GetNetwork(context.Background(), network)
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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
		invitePath := i.GetOperand("invite")

		req, err := parseInviteFile(invitePath)
		if err != nil {
			return fmt.Errorf("parse invite: %w", err)
		}

		client := api.NewClient(socketPath)
		result, err := client.InstallNetwork(context.Background(), req)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var clientNetworkRedeem = &args.Command{
	Name: "redeem",
	Help: "redeem an installed network's invite",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.RedeemNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

var clientNetworkConfirm = &args.Command{
	Name: "confirm",
	Help: "confirm a redeemed network's membership",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.ConfirmNetwork(context.Background(), network)
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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
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
		socketPath := i.GetParameterOr("socket-path", client.DefaultSocketPath)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.FetchNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		return printJSON(result)
	},
}

func parseInviteFile(
	path string,
) (
	api.InstallNetworkRequest,
	error,
) {
	data, err := os.ReadFile(path)
	if err != nil {
		return api.InstallNetworkRequest{}, err
	}

	var payload server.Invitation
	if err := json.Unmarshal(data, &payload); err != nil {
		return api.InstallNetworkRequest{}, err
	}

	return api.InstallNetworkRequest{
		NetworkName:          payload.Network.Name,
		InviteServerPubkey:   payload.Network.PublicKey,
		InviteServerEndpoint: payload.Network.Endpoint,
		InviteServerAddr:     payload.Network.APIEndpoint,
		TempPeerAssignedCidr: payload.Peer.CIDR,
		TempPeerPrivKey:      payload.Peer.PrivateKey,
	}, nil
}

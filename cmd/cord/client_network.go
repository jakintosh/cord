package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/pkg/invite"
)

var clientNetworkCmd = &args.Command{
	Name: "network",
	Help: "manage client networks",
	Subcommands: []*args.Command{
		clientNetworkList,
		clientNetworkShow,
		clientNetworkTopology,
		clientNetworkInstall,
		clientNetworkRedeem,
		clientNetworkConfirm,
		clientNetworkUninstall,
		clientNetworkEnable,
		clientNetworkDisable,
		clientNetworkSync,
		clientNetworkListenPort,
	},
}

var clientNetworkList = &args.Command{
	Name: "list",
	Help: "list installed networks",
	Handler: func(i *args.Input) error {
		client, err := clientClient(i)
		if err != nil {
			return err
		}

		networks, err := client.ListNetworks(i.Context())
		if err != nil {
			return err
		}

		return cliApp(i).Names(networks)
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
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		result, err := client.GetNetwork(i.Context(), network)
		if err != nil {
			return err
		}

		return cliApp(i).ClientNetworkDetail(result)
	},
}

var clientNetworkInstall = &args.Command{
	Name: "install",
	Help: "install a network from an invite",
	Operands: []args.Operand{
		{
			Name: "invite",
			Help: "path to the invite file, or - to read from stdin",
		},
	},
	Options: []args.Option{
		{
			Long: "listen-port",
			Type: args.OptionTypeParameter,
			Help: "local WireGuard UDP port (default: ephemeral)",
		},
	},
	Handler: func(i *args.Input) error {
		invitePath := i.GetOperand("invite")
		listenPortStr := i.GetParameter("listen-port")

		var (
			inviteBytes []byte
			err         error
		)
		if invitePath == "-" {
			inviteBytes, err = io.ReadAll(os.Stdin)
		} else {
			inviteBytes, err = os.ReadFile(invitePath)
		}
		if err != nil {
			return fmt.Errorf("read invite: %w", err)
		}

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		var port *uint16
		if listenPortStr != nil {
			if parsed, err := strconv.ParseUint(*listenPortStr, 10, 16); err != nil {
				return fmt.Errorf("invalid listen port %q", *listenPortStr)
			} else {
				listenPort := uint16(parsed)
				port = &listenPort
			}
		}

		invitation, err := invite.Parse(bytes.NewReader(inviteBytes))
		if err != nil {
			return fmt.Errorf("parse invite: %w", err)
		}

		result, err := client.InstallNetwork(
			i.Context(),
			*invitation,
			port,
		)
		if err != nil {
			return err
		}

		if result.Address != "" {
			return cliApp(i).Result(result, "network %q installed (%s)\n", result.Name, result.Address)
		}
		return cliApp(i).Result(result, "network %q installed\n", result.Name)
	},
}

var clientNetworkListenPort = &args.Command{
	Name: "listen-port",
	Help: "set a network's local WireGuard UDP port",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
		{
			Name: "port",
			Help: "UDP port, or 0 for ephemeral",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")
		portStr := i.GetOperand("port")

		var port uint16
		if parsed, err := strconv.ParseUint(portStr, 10, 16); err != nil {
			return fmt.Errorf("invalid listen port %q", portStr)
		} else {
			port = uint16(parsed)
		}

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		if _, err := client.UpdateNetwork(
			i.Context(),
			network,
			&port,
		); err != nil {
			return err
		}

		return cliApp(i).Message("network %q listen port set to %d\n", network, port)
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
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		result, err := client.RedeemNetwork(i.Context(), network)
		if err != nil {
			return err
		}

		if result.Address != "" {
			return cliApp(i).Result(result, "network %q redeemed (%s)\n", network, result.Address)
		}
		return cliApp(i).Result(result, "network %q redeemed\n", network)
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
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		if err := client.ConfirmNetwork(i.Context(), network); err != nil {
			return err
		}

		return cliApp(i).Message("network %q confirmed\n", network)
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
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		if err := client.UninstallNetwork(i.Context(), network); err != nil {
			return err
		}

		return cliApp(i).Message("network %q deleted\n", network)
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
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		status, err := client.EnableNetwork(i.Context(), network)
		if err != nil {
			return err
		}

		output := cliApp(i)
		if err := output.Message("network %q enabled\n", network); err != nil {
			return err
		}
		return output.ClientNetworkRuntime(status)
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
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		status, err := client.DisableNetwork(i.Context(), network)
		if err != nil {
			return err
		}

		output := cliApp(i)
		if err := output.Message("network %q disabled\n", network); err != nil {
			return err
		}
		return output.ClientNetworkRuntime(status)
	},
}

var clientNetworkSync = &args.Command{
	Name: "sync",
	Help: "sync peer state from server",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")

		client, err := clientClient(i)
		if err != nil {
			return err
		}

		result, err := client.SyncNetwork(i.Context(), network)
		if err != nil {
			return err
		}

		return cliApp(i).Result(result, "network %q synced\n", network)
	},
}

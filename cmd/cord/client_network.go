package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	adminclient "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
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

		if i.GetFlag("json") {
			return printJSON(networks)
		}

		for _, n := range networks {
			fmt.Println(n)
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

		if i.GetFlag("json") {
			return printJSON(result)
		}
		printClientNetworkDetail(result)
		return nil
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

		if i.GetFlag("json") {
			return printJSON(result)
		}
		if result.Address != "" {
			fmt.Printf("network %q installed (%s)\n", result.Name, result.Address)
		} else {
			fmt.Printf("network %q installed\n", result.Name)
		}
		return nil
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

		fmt.Printf("network %q listen port set to %d\n", network, port)
		return nil
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

		if i.GetFlag("json") {
			return printJSON(result)
		}
		if result.Address != "" {
			fmt.Printf("network %q redeemed (%s)\n", network, result.Address)
		} else {
			fmt.Printf("network %q redeemed\n", network)
		}
		return nil
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

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q confirmed\n", network)
		return nil
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

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q deleted\n", network)
		return nil
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

		if i.GetFlag("json") {
			return printJSON(status)
		}

		fmt.Printf("network %q enabled\n", network)
		printClientNetworkRuntime(status)
		return nil
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

		if i.GetFlag("json") {
			return printJSON(status)
		}

		fmt.Printf("network %q disabled\n", network)
		printClientNetworkRuntime(status)
		return nil
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

		if i.GetFlag("json") {
			return printJSON(result)
		}
		fmt.Printf("network %q synced\n", network)
		return nil
	},
}

// printClientNetworkRuntime reports what the daemon is actually doing
// with a network when that differs from the intent just recorded.
func printClientNetworkRuntime(
	status adminclient.NetworkStatus,
) {
	if status.Enabled == status.Running {
		return
	}
	if status.Reason == "" {
		fmt.Printf("running: %t\n", status.Running)
		return
	}
	fmt.Printf("running: %t (%s)\n", status.Running, status.Reason)
}

// printClientNetworkDetail prints the key: value detail view for a single
// network, as shown by `client network show`.
func printClientNetworkDetail(
	n adminclient.Network,
) {
	fmt.Printf("name: %s\n", n.Name)
	fmt.Printf("state: %s\n", n.State)
	fmt.Printf("enabled: %t\n", n.Enabled)
	if n.Address != "" {
		fmt.Printf("address: %s\n", n.Address)
	}
	if n.Interface != "" {
		fmt.Printf("interface: %s\n", n.Interface)
	}
	if n.ListenPort != 0 {
		fmt.Printf("listen_port: %d\n", n.ListenPort)
	}
	if n.ServerEndpoint != "" {
		fmt.Printf("server_endpoint: %s\n", n.ServerEndpoint)
	}
}

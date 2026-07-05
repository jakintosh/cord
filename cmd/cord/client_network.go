package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

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
		clientNetworkRedeem,
		clientNetworkConfirm,
		clientNetworkUninstall,
		clientNetworkEnable,
		clientNetworkDisable,
		clientNetworkSync,
	},
}

var clientNetworkList = &args.Command{
	Name: "list",
	Help: "list installed networks",
	Handler: func(i *args.Input) error {
		socketPath := clientSocket(i)

		client := api.NewClient(socketPath)
		networks, err := client.ListNetworks(context.Background())
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(networks)
		}

		printClientNetworks(networks)
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.GetNetwork(context.Background(), network)
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
	Handler: func(i *args.Input) error {
		socketPath := clientSocket(i)
		invitePath := i.GetOperand("invite")

		var (
			invite []byte
			err    error
		)
		if invitePath == "-" {
			invite, err = io.ReadAll(os.Stdin)
		} else {
			invite, err = os.ReadFile(invitePath)
		}
		if err != nil {
			return fmt.Errorf("read invite: %w", err)
		}

		client := api.NewClient(socketPath)
		result, err := client.InstallNetwork(context.Background(), invite)
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.RedeemNetwork(context.Background(), network)
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		if err := client.ConfirmNetwork(context.Background(), network); err != nil {
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		if err := client.UninstallNetwork(context.Background(), network); err != nil {
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		if err := client.EnableNetwork(context.Background(), network); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q enabled\n", network)
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		if err := client.DisableNetwork(context.Background(), network); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q disabled\n", network)
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
		socketPath := clientSocket(i)
		network := i.GetOperand("network")

		client := api.NewClient(socketPath)
		result, err := client.SyncNetwork(context.Background(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}
		fmt.Printf("network %q synced (%d peers)\n", network, result.PeerCount)
		return nil
	},
}

// printClientNetworks prints a one-row-per-network summary table.
func printClientNetworks(
	networks []api.NetworkDTO,
) {
	rows := make([][]string, len(networks))
	for idx, n := range networks {
		rows[idx] = []string{
			n.Name,
			n.State,
			strconv.FormatBool(n.Enabled),
			strconv.FormatBool(n.Connected),
		}
	}
	printTable([]string{"NAME", "STATE", "ENABLED", "CONNECTED"}, rows)
}

// printClientNetworkDetail prints the key: value detail view for a single
// network, as shown by `client network show`.
func printClientNetworkDetail(
	n api.NetworkDTO,
) {
	fmt.Printf("name: %s\n", n.Name)
	fmt.Printf("state: %s\n", n.State)
	fmt.Printf("enabled: %t\n", n.Enabled)
	fmt.Printf("connected: %t\n", n.Connected)
	if n.Address != "" {
		fmt.Printf("address: %s\n", n.Address)
	}
	if n.Interface != "" {
		fmt.Printf("interface: %s\n", n.Interface)
	}
	if n.ServerEndpoint != "" {
		fmt.Printf("server_endpoint: %s\n", n.ServerEndpoint)
	}
	fmt.Printf("peer_count: %d\n", n.PeerCount)
}

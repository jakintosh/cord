package main

import (
	"context"
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
)

var serverNetworkCmd = &args.Command{
	Name: "network",
	Help: "manage server networks",
	Subcommands: []*args.Command{
		serverNetworkAdd,
		serverNetworkDelete,
		serverNetworkList,
		serverNetworkShow,
		serverNetworkEnable,
		serverNetworkDisable,
	},
}

var serverNetworkAdd = &args.Command{
	Name: "add",
	Help: "create a server network",
	Operands: []args.Operand{
		{
			Name: "name",
			Help: "network name",
		},
		{
			Name: "main-cidr",
			Help: "root address range in CIDR notation",
		},
		{
			Name: "external-ip",
			Help: "external IP address for the WireGuard endpoint",
		},
	},
	Options: []args.Option{
		{
			Long: "main-wg-port",
			Type: args.OptionTypeParameter,
			Help: "WireGuard listen port for the main interface (default 51820)",
		},
		{
			Long: "main-api-port",
			Type: args.OptionTypeParameter,
			Help: "internal API port on the main tunnel (default 80)",
		},
		{
			Long: "main-name",
			Type: args.OptionTypeParameter,
			Help: "WireGuard interface name for the main device (default: network name)",
		},
		{
			Long: "invite-cidr",
			Type: args.OptionTypeParameter,
			Help: "CIDR for the invite interface (default 172.16.10.0/24)",
		},
		{
			Long: "invite-wg-port",
			Type: args.OptionTypeParameter,
			Help: "WireGuard listen port for the invite interface (default main port + 1)",
		},
		{
			Long: "invite-api-port",
			Type: args.OptionTypeParameter,
			Help: "internal API port on the invite tunnel (default 80)",
		},
		{
			Long: "invite-name",
			Type: args.OptionTypeParameter,
			Help: "WireGuard interface name for the invite device (default: network name + \"-i\")",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)

		req := admin.AddNetworkRequest{
			Name:          i.GetOperand("name"),
			ExternalIP:    i.GetOperand("external-ip"),
			MainName:      i.GetParameter("main-name"),
			MainCidr:      i.GetOperand("main-cidr"),
			MainWgPort:    toUint16Ptr(i.GetIntParameter("main-wg-port")),
			MainApiPort:   toUint16Ptr(i.GetIntParameter("main-api-port")),
			InviteName:    i.GetParameter("invite-name"),
			InviteCidr:    i.GetParameter("invite-cidr"),
			InviteApiPort: toUint16Ptr(i.GetIntParameter("invite-api-port")),
			InviteWgPort:  toUint16Ptr(i.GetIntParameter("invite-wg-port")),
		}

		client := admin.NewClient(socketPath)
		network, err := client.AddNetwork(context.Background(), req)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(network)
		}
		fmt.Printf(
			"network %q created (disabled); enable with 'cord server network enable %s'\n",
			req.Name, req.Name,
		)
		return nil
	},
}

var serverNetworkDelete = &args.Command{
	Name: "delete",
	Help: "delete a server network",
	Operands: []args.Operand{
		{
			Name: "name",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		if err := client.DeleteNetwork(context.Background(), name); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q deleted\n", name)
		return nil
	},
}

var serverNetworkList = &args.Command{
	Name: "list",
	Help: "list server networks",
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)

		client := admin.NewClient(socketPath)
		networks, err := client.ListNetworks(context.Background())
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

var serverNetworkShow = &args.Command{
	Name: "show",
	Help: "show a server network",
	Operands: []args.Operand{
		{
			Name: "name",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		network, err := client.ShowNetwork(context.Background(), name)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(network)
		}
		printServerNetworkDetail(network)
		return nil
	},
}

var serverNetworkEnable = &args.Command{
	Name: "enable",
	Help: "start a server network's WireGuard devices",
	Operands: []args.Operand{
		{
			Name: "name",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		if err := client.EnableNetwork(context.Background(), name); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q enabled\n", name)
		return nil
	},
}

var serverNetworkDisable = &args.Command{
	Name: "disable",
	Help: "stop a server network's WireGuard devices",
	Operands: []args.Operand{
		{
			Name: "name",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		if err := client.DisableNetwork(context.Background(), name); err != nil {
			return err
		}

		if i.GetFlag("json") {
			return nil
		}
		fmt.Printf("network %q disabled\n", name)
		return nil
	},
}

func toUint16Ptr(v *int) *uint16 {
	if v == nil {
		return nil
	}
	u := uint16(*v)
	return &u
}

// printServerNetworkDetail prints the key: value detail view for a single
// network, as shown by `server network show`.
func printServerNetworkDetail(
	n admin.NetworkDTO,
) {
	fmt.Printf("name: %s\n", n.Name)
	fmt.Printf("external_ip: %s\n", n.ExternalIP)
	fmt.Printf("main_name: %s\n", n.MainName)
	fmt.Printf("main_cidr: %s\n", n.MainCidr)
	fmt.Printf("main_wg_port: %d\n", n.MainWgPort)
	fmt.Printf("main_api_port: %d\n", n.MainApiPort)
	fmt.Printf("invite_name: %s\n", n.InviteName)
	fmt.Printf("invite_cidr: %s\n", n.InviteCidr)
	fmt.Printf("invite_wg_port: %d\n", n.InviteWgPort)
	fmt.Printf("invite_api_port: %d\n", n.InviteApiPort)
	fmt.Printf("enabled: %t\n", n.Enabled)
}

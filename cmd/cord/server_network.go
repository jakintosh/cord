package main

import (
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
		serverNetworkTopology,
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
			Help: "internal API port on the main tunnel (default 8080)",
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
			Help: "internal API port on the invite tunnel (default 8080)",
		},
		{
			Long: "invite-name",
			Type: args.OptionTypeParameter,
			Help: "WireGuard interface name for the invite device (default: network name + \"-i\")",
		},
	},
	Handler: func(i *args.Input) error {
		req := admin.CreateNetworkRequest{
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

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		network, err := client.AddNetwork(i.Context(), req)
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
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		if err := client.DeleteNetwork(i.Context(), name); err != nil {
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
		client, err := serverClient(i)
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
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		network, err := client.ShowNetwork(i.Context(), name)
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
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		status, err := client.EnableNetwork(i.Context(), name)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(status)
		}

		fmt.Printf("network %q enabled\n", name)
		printNetworkRuntime(status)
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
		name := i.GetOperand("name")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		status, err := client.DisableNetwork(i.Context(), name)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(status)
		}

		fmt.Printf("network %q disabled\n", name)
		printNetworkRuntime(status)
		return nil
	},
}

// printNetworkRuntime reports what the daemon is actually doing with a
// network when that differs from the intent just recorded.
func printNetworkRuntime(
	status admin.NetworkStatus,
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
	n admin.Network,
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

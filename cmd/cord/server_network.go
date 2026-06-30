package main

import (
	"context"
	"fmt"
	"os"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)

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

		if err := printJSON(network); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Network created and disabled. Use 'cord server network enable %s' to start it.\n", req.Name)
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		if err := client.DeleteNetwork(context.Background(), name); err != nil {
			return err
		}

		fmt.Println("ok")
		return nil
	},
}

var serverNetworkList = &args.Command{
	Name:    "list",
	Help:    "list server networks",
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)

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
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		network, err := client.ShowNetwork(context.Background(), name)
		if err != nil {
			return err
		}

		return printJSON(network)
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		if err := client.EnableNetwork(context.Background(), name); err != nil {
			return err
		}

		fmt.Println("enabled")
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
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		name := i.GetOperand("name")

		client := admin.NewClient(socketPath)
		if err := client.DisableNetwork(context.Background(), name); err != nil {
			return err
		}

		fmt.Println("disabled")
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

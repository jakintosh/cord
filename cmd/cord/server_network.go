package main

import (
	"git.sr.ht/~jakintosh/command-go/pkg/args"
	adminserver "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
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
			Long: "main-name",
			Type: args.OptionTypeParameter,
			Help: "WireGuard interface name for the main device (default: network name)",
		},
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
			Long: "invite-cidr",
			Type: args.OptionTypeParameter,
			Help: "CIDR for the invite interface (default 172.16.10.0/24)",
		},
		{
			Long: "invite-name",
			Type: args.OptionTypeParameter,
			Help: "WireGuard interface name for the invite device (default: network name + \"-i\")",
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
	},
	Handler: func(i *args.Input) error {
		req := adminserver.CreateNetworkRequest{
			Name:          i.GetOperand("name"),
			ExternalIP:    i.GetOperand("external-ip"),
			MainCidr:      i.GetOperand("main-cidr"),
			MainName:      i.GetParameter("main-name"),
			MainWgPort:    toUint16Ptr(i.GetIntParameter("main-wg-port")),
			MainApiPort:   toUint16Ptr(i.GetIntParameter("main-api-port")),
			InviteCidr:    i.GetParameter("invite-cidr"),
			InviteName:    i.GetParameter("invite-name"),
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

		return cliApp(i).Result(network,
			"network %q created (disabled); enable with 'cord server network enable %s'\n",
			req.Name, req.Name,
		)
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

		return cliApp(i).Message("network %q deleted\n", name)
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

		return cliApp(i).Names(networks)
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

		return cliApp(i).ServerNetworkDetail(network)
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

		output := cliApp(i)
		if err := output.Message("network %q enabled\n", name); err != nil {
			return err
		}
		return output.ServerNetworkRuntime(status)
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

		output := cliApp(i)
		if err := output.Message("network %q disabled\n", name); err != nil {
			return err
		}
		return output.ServerNetworkRuntime(status)
	},
}

func toUint16Ptr(v *int) *uint16 {
	if v == nil {
		return nil
	}
	u := uint16(*v)
	return &u
}

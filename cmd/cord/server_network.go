package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
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
			Name: "cidr",
			Help: "root address range in CIDR notation",
		},
		{
			Name: "external-ip",
			Help: "external IP address for the WireGuard endpoint",
		},
		{
			Name: "port",
			Help: "WireGuard listen port",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", server.DefaultSocketPath)
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")
		externalIP := i.GetOperand("external-ip")
		portStr := i.GetOperand("port")

		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}

		client := api.NewClient(socketPath)
		network, err := client.AddNetwork(context.Background(), api.AddNetworkRequest{
			Name:       name,
			Cidr:       cidr,
			ExternalIP: externalIP,
			Port:       uint16(port),
		})
		if err != nil {
			return err
		}

		if err := printJSON(network); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Network created and disabled. Use 'cord server network enable %s' to start it.\n", name)
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

		client := api.NewClient(socketPath)
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

		client := api.NewClient(socketPath)
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

		client := api.NewClient(socketPath)
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

		client := api.NewClient(socketPath)
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

		client := api.NewClient(socketPath)
		if err := client.DisableNetwork(context.Background(), name); err != nil {
			return err
		}

		fmt.Println("disabled")
		return nil
	},
}

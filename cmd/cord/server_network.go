package main

import (
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

var serverNetworkCmd = &args.Command{
	Name: "network",
	Help: "manage cord networks",
	Subcommands: []*args.Command{
		serverNetworkAdd,
		serverNetworkDelete,
		serverNetworkList,
		serverNetworkShow,
	},
}

var serverNetworkAdd = &args.Command{
	Name: "add",
	Help: "create a new cord",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name for the new network",
		},
		{
			Name: "cidr",
			Help: "root CIDR for the new network",
		},
		{
			Name: "external-ip",
			Help: "external IP the coordination server can be reached at",
		},
		{
			Name: "external-port",
			Help: "external port the coordination server listens on",
		},
	},
	Options: []args.Option{
		{
			Long: "invite-cidr",
			Type: args.OptionTypeParameter,
			Help: "CIDR for the invite network (default " + DEFAULT_INVITE_CIDR + ")",
		},
		{
			Long: "invite-port",
			Type: args.OptionTypeParameter,
			Help: "external port for the invite network (default: external-port + 1)",
		},
		{
			Long: "api-port",
			Type: args.OptionTypeParameter,
			Help: "internal TCP port for the HTTP API (default: external-port)",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		cidrValue := i.GetOperand("cidr")
		ipValue := i.GetOperand("external-ip")
		portValue := i.GetOperand("external-port")

		// options
		inviteCidrValue := i.GetParameterOr("invite-cidr", DEFAULT_INVITE_CIDR)

		// parse
		if err := server.ValidateNetworkName(network); err != nil {
			return err
		}

		cidr, err := parseCidr(cidrValue)
		if err != nil {
			return fmt.Errorf("failed to parse cidr: %w", err)
		}

		inviteCidr, err := parseCidr(inviteCidrValue)
		if err != nil {
			return fmt.Errorf("failed to parse invite cidr: %w", err)
		}

		ip, err := parseIp(ipValue)
		if err != nil {
			return fmt.Errorf("failed to parse ip: %w", err)
		}

		port, err := parsePort(portValue)
		if err != nil {
			return fmt.Errorf("failed to parse port: %w", err)
		}

		invitePort := uint16(i.GetIntParameterOr("invite-port", int(port)+1))
		apiPort := uint16(i.GetIntParameterOr("api-port", int(port)))

		// create server
		srv, err := newServerCreate(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.CreateNetwork(server.CreateNetworkRequest{
			RootCidr:   cidr,
			InviteCidr: inviteCidr,
			ExternalIP: ip,
			ListenPort: port,
			InvitePort: invitePort,
			ApiPort:    apiPort,
		})
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}

		fmt.Printf("created network '%s' (%s)\n", network, cidr.String())
		return nil
	},
}

var serverNetworkDelete = &args.Command{
	Name: "delete",
	Help: "delete an existing cord",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of the network to delete",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.DeleteNetwork()
		if err != nil {
			return fmt.Errorf("failed to delete network: %w", err)
		}

		return nil
	},
}

var serverNetworkList = &args.Command{
	Name:    "list",
	Help:    "list all networks on this server",
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// options
		jsonOut := i.GetFlag("json")

		// list networks straight from the config directory; no
		// database is opened and nothing is created
		configDir, _ := serverDirs(i)
		summaries, err := server.ListNetworks(server.NewFsConfig(configDir))
		if err != nil {
			return fmt.Errorf("failed to list networks: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(summaries)
		}
		if len(summaries) == 0 {
			fmt.Println("no networks found")
			return nil
		}
		rows := make([][]string, 0, len(summaries))
		for _, summary := range summaries {
			rows = append(rows, []string{
				summary.Name,
				summary.RootCidr,
				summary.ExternalEndpoint,
			})
		}
		printTable([]string{"NAME", "ROOT CIDR", "ENDPOINT"}, rows)
		return nil
	},
}

var serverNetworkShow = &args.Command{
	Name: "show",
	Help: "show a network's configuration and resource counts",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of the network to show",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return err
		}

		// execute
		overview, err := srv.GetNetworkOverview()
		if err != nil {
			return fmt.Errorf("failed to get network overview: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(overview)
		}
		fmt.Printf("name:            %s\n", overview.Name)
		fmt.Printf("public key:      %s\n", overview.PublicKey)
		fmt.Printf("root cidr:       %s\n", overview.RootCidr)
		fmt.Printf("invite cidr:     %s\n", overview.InviteCidr)
		fmt.Printf("external ip:     %s\n", overview.ExternalIP)
		fmt.Printf("listen port:     %d\n", overview.ListenPort)
		fmt.Printf("invite port:     %d\n", overview.InviteListenPort)
		fmt.Printf("api port:        %d\n", overview.ApiPort)
		fmt.Println()
		fmt.Printf("cidrs:           %d\n", overview.CidrCount)
		fmt.Printf("peers:           %d\n", overview.PeerCount)
		fmt.Printf("active invites:  %d\n", overview.ActiveInvites)
		fmt.Printf("associations:    %d\n", overview.AssociationCount)
		return nil
	},
}

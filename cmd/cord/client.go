package main

import (
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/client"
	"git.sr.ht/~jakintosh/cord/internal/database"
)

func clientDirs(i *args.Input) (string, string) {
	configDir := i.GetParameterOr("config-dir", CLIENT_DEFAULT_CFG)
	dataDir := i.GetParameterOr("data-dir", CLIENT_DEFAULT_DATA)
	return configDir, dataDir
}

// newClient opens an installed network; it errors without creating any
// state if the network's config is missing. The peer database is a
// recreatable cache, so it is created as needed.
func newClient(
	i *args.Input,
	network string,
) (
	*client.Client,
	error,
) {
	configDir, dataDir := clientDirs(i)
	if err := client.RequireInstalled(configDir, network); err != nil {
		return nil, err
	}
	return openClient(configDir, dataDir, network, i.GetFlag("verbose"))
}

// newClientCreate opens a network for installation, creating
// directories and the peer database as needed.
func newClientCreate(
	i *args.Input,
	network string,
) (
	*client.Client,
	error,
) {
	configDir, dataDir := clientDirs(i)
	return openClient(configDir, dataDir, network, i.GetFlag("verbose"))
}

func openClient(
	configDir string,
	dataDir string,
	network string,
	verbose bool,
) (
	*client.Client,
	error,
) {
	dbOpts := database.Options{
		Name: network,
		Dir:  dataDir,
	}
	store, err := database.OpenClient(dbOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open peer store: %w", err)
	}

	opts := client.Options{
		Network:   network,
		ConfigDir: configDir,
		DataDir:   dataDir,
		Store:     store,
		Verbose:   verbose,
	}
	return client.New(opts)
}

var clientCmd = &args.Command{
	Name: "client",
	Help: "join and run cord networks on this machine",
	Subcommands: []*args.Command{
		clientInstall,
		clientUninstall,
		clientShow,
		clientFetch,
		clientUp,
		clientDown,
		adminCmd,
	},
}

var clientInstall = &args.Command{
	Name: "install",
	Help: "redeem and install a cord peer invite",
	Operands: []args.Operand{
		{
			Name: "invite",
			Help: "the invite file for a cord network",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		invitePath := i.GetOperand("invite")

		// the network name comes from the invite
		invite, err := client.LoadInvite(invitePath)
		if err != nil {
			return err
		}

		c, err := newClientCreate(i, invite.Interface.NetworkName)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		if err := c.Install(invite); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var clientUninstall = &args.Command{
	Name: "uninstall",
	Help: "uninstall a cord network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// create client
		c, err := newClient(i, network)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		if err := c.Uninstall(); err != nil {
			return fmt.Errorf("failed to uninstall: %w", err)
		}

		return nil
	},
}

var clientShow = &args.Command{
	Name: "show",
	Help: "show information for cord networks",
	Options: []args.Option{
		{
			Short: 'n',
			Long:  "network",
			Type:  args.OptionTypeParameter,
			Help:  "the network to show information for",
		},
	},
	Handler: func(i *args.Input) error {

		// options
		network := i.GetParameterOr("network", "")

		// with no network selected, list the installed networks
		if network == "" {
			configDir, _ := clientDirs(i)
			return client.ShowInstalled(configDir)
		}

		c, err := newClient(i, network)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		if err := c.Show(); err != nil {
			return fmt.Errorf("failed to show: %w", err)
		}

		return nil
	},
}

var clientFetch = &args.Command{
	Name: "fetch",
	Help: "update cord state from the server",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// create client
		c, err := newClient(i, network)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		if err := c.Fetch(); err != nil {
			return fmt.Errorf("failed to fetch: %w", err)
		}

		return nil
	},
}

var clientUp = &args.Command{
	Name: "up",
	Help: "connect the wireguard interface for a cord (runs in the foreground)",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []args.Option{
		{
			Long: "no-fetch",
			Type: args.OptionTypeFlag,
			Help: "do not fetch peer state from the server",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		noFetch := i.GetFlag("no-fetch")

		// create client
		c, err := newClient(i, network)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		if err := c.Up(noFetch); err != nil {
			return fmt.Errorf("failed to bring cord up: %w", err)
		}

		return nil
	},
}

var clientDown = &args.Command{
	Name: "down",
	Help: "disable the wireguard interface for a cord",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// create client
		c, err := newClient(i, network)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		if err := c.Down(); err != nil {
			return fmt.Errorf("failed to bring cord down: %w", err)
		}

		return nil
	},
}

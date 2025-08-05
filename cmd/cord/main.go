package main

import (
	"fmt"

	cmd "git.sr.ht/~jakintosh/command-go"
	"git.sr.ht/~jakintosh/cord/internal/client"
)

const (
	BIN_NAME     = "cord"
	AUTHOR       = "jakintosh"
	VERSION      = "0.1"
	DEFAULT_CFG  = "/etc/" + BIN_NAME
	DEFAULT_DATA = "/var/lib/" + BIN_NAME
)

func main() {
	root.Parse()
}

var root = &cmd.Command{
	Name:    BIN_NAME,
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "map cords to wireguard interfaces",
	Subcommands: []*cmd.Command{
		install,
		uninstall,
		show,
		fetch,
		up,
		down,
		server,
	},
	Operands: []cmd.Operand{},
	Options: []cmd.Option{
		{
			Short: 0,
			Long:  "config-dir",
			Type:  cmd.OptionTypeParameter,
			Help:  "directory for config files",
		},
		{
			Short: 0,
			Long:  "data-dir",
			Type:  cmd.OptionTypeParameter,
			Help:  "directory for program data",
		},
	},
}

var install = &cmd.Command{
	Name:        "install",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "redeem and install an cord peer invite",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "invite",
			Help: "the invite file for an cord network",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		//operands
		invite := i.GetOperand("invite")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := client.NewContext("", configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Install(invite); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var uninstall = &cmd.Command{
	Name:        "uninstall",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "uninstall an cord network",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := client.NewContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Uninstall(); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var show = &cmd.Command{
	Name:        "show",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "show information for cord networks",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options: []cmd.Option{
		{
			Short: 'n',
			Long:  "network",
			Type:  cmd.OptionTypeParameter,
			Help:  "the network to show information for",
		},
	},
	Handler: func(i *cmd.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)
		network := i.GetParameterOr("network", "")

		// create app context
		ctx, err := client.NewContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Show(); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var fetch = &cmd.Command{
	Name:        "fetch",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "update cord state from the server",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := client.NewContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Fetch(); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var up = &cmd.Command{
	Name:        "up",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "enable the wireguard interface for an cord",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := client.NewContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Up(); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var down = &cmd.Command{
	Name:        "down",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "disable the wireguard interface for an cord",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := client.NewContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Down(); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var server = &cmd.Command{
	Name:    "server",
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "manage an cord server over its HTTP API",
	Subcommands: []*cmd.Command{
		peer,
		cidr,
		association,
	},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []cmd.Option{},
}

var peer = &cmd.Command{
	Name:    "peer",
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "manage peers",
	Subcommands: []*cmd.Command{
		addPeer,
		renamePeer,
		enablePeer,
		disablePeer,
	},
	Operands: []cmd.Operand{},
	Options:  []cmd.Option{},
}

var addPeer = &cmd.Command{
	Name:        "add",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a peer invite for an cord",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("POST http://cord-server:port/api/v1/admin/peer\n")
		return nil
	},
}

var renamePeer = &cmd.Command{
	Name:        "rename",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "rename a peer",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("PUT http://cord-server:port/api/v1/admin/peer\n")
		return nil
	},
}

var enablePeer = &cmd.Command{
	Name:        "enable",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "set a peer to enabled",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("PUT http://cord-server:port/api/v1/admin/peer\n")
		return nil
	},
}

var disablePeer = &cmd.Command{
	Name:        "disable",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "set a peer to disabled",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("PUT http://cord-server:port/api/v1/admin/peer\n")
		return nil
	},
}

var cidr = &cmd.Command{
	Name:    "cidr",
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "manage cidrs",
	Subcommands: []*cmd.Command{
		addCidr,
		renameCidr,
		deleteCidr,
	},
	Operands: []cmd.Operand{},
	Options:  []cmd.Option{},
}

var addCidr = &cmd.Command{
	Name:        "add",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a cidr",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("POST http://cord-server:port/api/v1/admin/cidr\n")
		return nil
	},
}

var renameCidr = &cmd.Command{
	Name:        "rename",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "rename a cidr",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("PUT http://cord-server:port/api/v1/admin/cidr\n")
		return nil
	},
}

var deleteCidr = &cmd.Command{
	Name:        "delete",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete a cidr",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("DELETE http://cord-server:port/api/v1/admin/cidr\n")
		return nil
	},
}

var association = &cmd.Command{
	Name:    "association",
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "manage associations",
	Subcommands: []*cmd.Command{
		addAssociation,
		deleteAssociation,
	},
	Operands: []cmd.Operand{},
	Options:  []cmd.Option{},
}

var addAssociation = &cmd.Command{
	Name:        "add",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a association",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("POST http://cord-server:port/api/v1/admin/association\n")
		return nil
	},
}

var deleteAssociation = &cmd.Command{
	Name:        "delete",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete a association",
	Subcommands: []*cmd.Command{},
	Operands:    []cmd.Operand{},
	Options:     []cmd.Option{},
	Handler: func(*cmd.Input) error {

		fmt.Printf("DELETE http://cord-server:port/api/v1/admin/association\n")
		return nil
	},
}

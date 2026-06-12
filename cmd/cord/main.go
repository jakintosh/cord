package main

import (
	"fmt"
	"os"
	"path"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/client"
)

const (
	BIN_NAME     = "cord"
	AUTHOR       = "jakintosh"
	VERSION      = "0.2"
	DEFAULT_CFG  = "/etc/" + BIN_NAME
	DEFAULT_DATA = "/var/lib/" + BIN_NAME
)

func main() {
	root.Parse()
}

func newContext(
	i *args.Input,
	network string,
) (
	*client.Context,
	error,
) {
	configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
	dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)
	return client.NewContext(network, configDir, dataDir)
}

var root = &args.Command{
	Name: BIN_NAME,
	Help: "map cords to wireguard interfaces",
	Config: &args.Config{
		Author:  AUTHOR,
		Version: VERSION,
		HelpOption: &args.HelpOption{
			Short: 'h',
			Long:  "help",
		},
	},
	Subcommands: []*args.Command{
		install,
		uninstall,
		show,
		fetch,
		up,
		down,
		serverCmd,
	},
	Options: []args.Option{
		{
			Long: "config-dir",
			Type: args.OptionTypeParameter,
			Help: "directory for config files",
		},
		{
			Long: "data-dir",
			Type: args.OptionTypeParameter,
			Help: "directory for program data",
		},
	},
}

var install = &args.Command{
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
		invite := i.GetOperand("invite")

		// create client context; the network name comes from the invite
		ctx, err := newContext(i, "")
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Install(invite); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		return nil
	},
}

var uninstall = &args.Command{
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

		// create client context
		ctx, err := newContext(i, network)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Uninstall(); err != nil {
			return fmt.Errorf("failed to uninstall: %w", err)
		}

		return nil
	},
}

var show = &args.Command{
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

		// create client context
		ctx, err := newContext(i, network)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Show(); err != nil {
			return fmt.Errorf("failed to show: %w", err)
		}

		return nil
	},
}

var fetch = &args.Command{
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

		// create client context
		ctx, err := newContext(i, network)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Fetch(); err != nil {
			return fmt.Errorf("failed to fetch: %w", err)
		}

		return nil
	},
}

var up = &args.Command{
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

		// create client context
		ctx, err := newContext(i, network)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Up(noFetch); err != nil {
			return fmt.Errorf("failed to bring cord up: %w", err)
		}

		return nil
	},
}

var down = &args.Command{
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

		// create client context
		ctx, err := newContext(i, network)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		if err := ctx.Down(); err != nil {
			return fmt.Errorf("failed to bring cord down: %w", err)
		}

		return nil
	},
}

// remote administration over the server's HTTP API; requires this
// machine to be an admin peer on the network

func adminFor(i *args.Input, network string) (*client.Admin, error) {
	ctx, err := newContext(i, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}
	admin, err := ctx.Admin()
	if err != nil {
		return nil, err
	}
	return admin, nil
}

var serverCmd = &args.Command{
	Name: "server",
	Help: "manage a cord server over its HTTP API (requires admin peer)",
	Subcommands: []*args.Command{
		peerCmd,
		cidrCmd,
		associationCmd,
	},
}

var peerCmd = &args.Command{
	Name: "peer",
	Help: "manage peers",
	Subcommands: []*args.Command{
		addPeer,
		renamePeer,
		enablePeer,
		disablePeer,
		deletePeer,
	},
}

var addPeer = &args.Command{
	Name: "add",
	Help: "create a peer invite for a cord",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "name",
			Help: "name of the new peer",
		},
		{
			Name: "ip",
			Help: "IP of the new peer",
		},
	},
	Options: []args.Option{
		{
			Short: 'a',
			Long:  "admin",
			Type:  args.OptionTypeFlag,
			Help:  "make new peer an admin?",
		},
		{
			Long: "save-invite",
			Type: args.OptionTypeParameter,
			Help: "directory to write the invite to",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		ip := i.GetOperand("ip")

		// options
		isAdmin := i.GetFlag("admin")
		saveDir := i.GetParameterOr("save-invite", "")
		if saveDir == "" {
			var err error
			saveDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("couldn't read pwd: %w", err)
			}
		}

		admin, err := adminFor(i, network)
		if err != nil {
			return err
		}

		invite, err := admin.AddPeer(name, ip, isAdmin, 0)
		if err != nil {
			return fmt.Errorf("failed to add peer: %w", err)
		}

		savePath := path.Join(saveDir, name+".invite.toml")
		file, err := os.OpenFile(savePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("failed to open file '%s': %w", savePath, err)
		}
		defer file.Close()

		if err := invite.Write(file); err != nil {
			return fmt.Errorf("failed to write invite: %w", err)
		}

		fmt.Printf("wrote invite for '%s' to %s\n", name, savePath)
		return nil
	},
}

var renamePeer = &args.Command{
	Name: "rename",
	Help: "rename a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "peer",
			Help: "peer to rename",
		},
		{
			Name: "new-name",
			Help: "new name for peer",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		_, err = admin.RenamePeer(i.GetOperand("peer"), i.GetOperand("new-name"))
		if err != nil {
			return fmt.Errorf("failed to rename peer: %w", err)
		}

		return nil
	},
}

var enablePeer = &args.Command{
	Name: "enable",
	Help: "set a peer to enabled",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "peer",
			Help: "peer to enable",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		if _, err := admin.EnablePeer(i.GetOperand("peer")); err != nil {
			return fmt.Errorf("failed to enable peer: %w", err)
		}

		return nil
	},
}

var disablePeer = &args.Command{
	Name: "disable",
	Help: "set a peer to disabled",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "peer",
			Help: "peer to disable",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		if _, err := admin.DisablePeer(i.GetOperand("peer")); err != nil {
			return fmt.Errorf("failed to disable peer: %w", err)
		}

		return nil
	},
}

var deletePeer = &args.Command{
	Name: "delete",
	Help: "delete a peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "peer",
			Help: "peer to delete",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		if err := admin.DeletePeer(i.GetOperand("peer")); err != nil {
			return fmt.Errorf("failed to delete peer: %w", err)
		}

		return nil
	},
}

var cidrCmd = &args.Command{
	Name: "cidr",
	Help: "manage cidrs",
	Subcommands: []*args.Command{
		addCidr,
		renameCidr,
		deleteCidr,
	},
}

var addCidr = &args.Command{
	Name: "add",
	Help: "create a cidr",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "name",
			Help: "name of the CIDR",
		},
		{
			Name: "cidr",
			Help: "address range in CIDR notation",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		_, err = admin.AddCidr(i.GetOperand("name"), i.GetOperand("cidr"))
		if err != nil {
			return fmt.Errorf("failed to add cidr: %w", err)
		}

		return nil
	},
}

var renameCidr = &args.Command{
	Name: "rename",
	Help: "rename a cidr",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "cidr",
			Help: "CIDR to rename",
		},
		{
			Name: "new-name",
			Help: "new name for CIDR",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		_, err = admin.RenameCidr(i.GetOperand("cidr"), i.GetOperand("new-name"))
		if err != nil {
			return fmt.Errorf("failed to rename cidr: %w", err)
		}

		return nil
	},
}

var deleteCidr = &args.Command{
	Name: "delete",
	Help: "delete a cidr",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "cidr",
			Help: "CIDR to delete",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		if err := admin.DeleteCidr(i.GetOperand("cidr")); err != nil {
			return fmt.Errorf("failed to delete cidr: %w", err)
		}

		return nil
	},
}

var associationCmd = &args.Command{
	Name: "association",
	Help: "manage associations",
	Subcommands: []*args.Command{
		addAssociation,
		deleteAssociation,
	},
}

var addAssociation = &args.Command{
	Name: "add",
	Help: "create an association",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "cidr1",
			Help: "name of the first CIDR",
		},
		{
			Name: "cidr2",
			Help: "name of the second CIDR",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		err = admin.AddAssociation(i.GetOperand("cidr1"), i.GetOperand("cidr2"))
		if err != nil {
			return fmt.Errorf("failed to add association: %w", err)
		}

		return nil
	},
}

var deleteAssociation = &args.Command{
	Name: "delete",
	Help: "delete an association",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
		{
			Name: "cidr1",
			Help: "name of the first CIDR",
		},
		{
			Name: "cidr2",
			Help: "name of the second CIDR",
		},
	},
	Handler: func(i *args.Input) error {

		admin, err := adminFor(i, i.GetOperand("network"))
		if err != nil {
			return err
		}

		err = admin.DeleteAssociation(i.GetOperand("cidr1"), i.GetOperand("cidr2"))
		if err != nil {
			return fmt.Errorf("failed to delete association: %w", err)
		}

		return nil
	},
}

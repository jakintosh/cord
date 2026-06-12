package main

import (
	"fmt"
	"os"
	"path"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/client"
)

// remote administration over the server's HTTP API; requires this
// machine to be an admin peer on the network

func adminFor(i *args.Input, network string) (*client.Admin, error) {
	c, err := newClient(i, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	admin, err := c.Admin()
	if err != nil {
		return nil, err
	}
	return admin, nil
}

var adminCmd = &args.Command{
	Name: "admin",
	Help: "manage a cord server over its HTTP API (requires admin peer)",
	Subcommands: []*args.Command{
		adminPeerCmd,
		adminCidrCmd,
		adminAssociationCmd,
		adminInviteCmd,
	},
}

var adminPeerCmd = &args.Command{
	Name: "peer",
	Help: "manage peers",
	Subcommands: []*args.Command{
		adminPeerAdd,
		adminPeerRename,
		adminPeerEnable,
		adminPeerDisable,
		adminPeerDelete,
		adminPeerList,
	},
}

var adminPeerAdd = &args.Command{
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

var adminPeerRename = &args.Command{
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

var adminPeerEnable = &args.Command{
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

var adminPeerDisable = &args.Command{
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

var adminPeerDelete = &args.Command{
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

var adminPeerList = &args.Command{
	Name: "list",
	Help: "list a network's peers and their status",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create admin client
		admin, err := adminFor(i, network)
		if err != nil {
			return err
		}

		// execute
		peers, err := admin.ListPeers()
		if err != nil {
			return fmt.Errorf("failed to list peers: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(peers)
		}
		rows := make([][]string, 0, len(peers))
		for _, peer := range peers {
			rows = append(rows, []string{
				peer.Name,
				peer.Cidr,
				yesNo(peer.Admin),
				yesNo(peer.Enabled),
				yesNo(peer.Confirmed),
				truncateKey(peer.PublicKey),
			})
		}
		printTable(
			[]string{"NAME", "CIDR", "ADMIN", "ENABLED", "CONFIRMED", "PUBLIC KEY"},
			rows,
		)
		return nil
	},
}

var adminCidrCmd = &args.Command{
	Name: "cidr",
	Help: "manage cidrs",
	Subcommands: []*args.Command{
		adminCidrAdd,
		adminCidrRename,
		adminCidrDelete,
		adminCidrList,
	},
}

var adminCidrAdd = &args.Command{
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

var adminCidrRename = &args.Command{
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

var adminCidrDelete = &args.Command{
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

var adminCidrList = &args.Command{
	Name: "list",
	Help: "list a network's cidrs",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create admin client
		admin, err := adminFor(i, network)
		if err != nil {
			return err
		}

		// execute
		cidrs, err := admin.ListCidrs()
		if err != nil {
			return fmt.Errorf("failed to list cidrs: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(cidrs)
		}
		rows := make([][]string, 0, len(cidrs))
		for _, cidr := range cidrs {
			rows = append(rows, []string{
				cidr.Name,
				cidr.Cidr,
			})
		}
		printTable([]string{"NAME", "CIDR"}, rows)
		return nil
	},
}

var adminAssociationCmd = &args.Command{
	Name: "association",
	Help: "manage associations",
	Subcommands: []*args.Command{
		adminAssociationAdd,
		adminAssociationDelete,
		adminAssociationList,
	},
}

var adminAssociationAdd = &args.Command{
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

var adminAssociationDelete = &args.Command{
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

var adminAssociationList = &args.Command{
	Name: "list",
	Help: "list a network's CIDR associations",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create admin client
		admin, err := adminFor(i, network)
		if err != nil {
			return err
		}

		// execute
		associations, err := admin.ListAssociations()
		if err != nil {
			return fmt.Errorf("failed to list associations: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(associations)
		}
		rows := make([][]string, 0, len(associations))
		for _, association := range associations {
			rows = append(rows, []string{
				association.Cidr1,
				association.Cidr2,
			})
		}
		printTable([]string{"CIDR1", "CIDR2"}, rows)
		return nil
	},
}

var adminInviteCmd = &args.Command{
	Name: "invite",
	Help: "inspect invites",
	Subcommands: []*args.Command{
		adminInviteList,
	},
}

var adminInviteList = &args.Command{
	Name: "list",
	Help: "list a network's invites and their state",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of cord",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create admin client
		admin, err := adminFor(i, network)
		if err != nil {
			return err
		}

		// execute
		invites, err := admin.ListInvites()
		if err != nil {
			return fmt.Errorf("failed to list invites: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(invites)
		}
		printInviteTable(invites)
		return nil
	},
}

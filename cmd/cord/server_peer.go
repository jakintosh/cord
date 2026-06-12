package main

import (
	"fmt"
	"os"
	"path"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

var serverPeerCmd = &args.Command{
	Name: "peer",
	Help: "manage a network's peers",
	Subcommands: []*args.Command{
		serverPeerAdd,
		serverPeerRename,
		serverPeerEnable,
		serverPeerDisable,
		serverPeerDelete,
		serverPeerList,
		serverPeerVisible,
	},
}

var serverPeerAdd = &args.Command{
	Name: "add",
	Help: "create a new peer invite",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to add peer to",
		},
		{
			Name: "name",
			Help: "name of the peer",
		},
		{
			Name: "ip",
			Help: "IP of peer (immutable once created)",
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
		{
			Long: "invite-expires",
			Type: args.OptionTypeParameter,
			Help: "invite expiration period (eg. '30d', '7w', '2h', '1000s')",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		ipValue := i.GetOperand("ip")

		// options
		admin := i.GetFlag("admin")
		savePath := i.GetParameterOr("save-invite", getPwd())
		inviteValue := i.GetParameterOr("invite-expires", "7d")

		// parse
		ip, err := parseIp(ipValue)
		if err != nil {
			return fmt.Errorf("failed to parse ip: %w", err)
		}

		expiration, err := parseExpiration(inviteValue)
		if err != nil {
			return fmt.Errorf("failed to parse expiration: %w", err)
		}

		// make sure we have a file handle before db logic
		fileName := name + ".invite.toml"
		savePath = path.Join(savePath, fileName)
		inviteFile, err := os.OpenFile(savePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return fmt.Errorf("failed to open file '%s': %w", savePath, err)
		}
		defer inviteFile.Close()

		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		req := server.CreateInviteRequest{
			Name:       name,
			IP:         ip,
			Admin:      admin,
			Expiration: expiration,
		}
		invite, err := srv.CreateInvite(req)
		if err != nil {
			return fmt.Errorf("failed to create peer: %w", err)
		}

		err = invite.Write(inviteFile)
		if err != nil {
			return fmt.Errorf("failed to write invite: %w", err)
		}

		fmt.Printf("wrote invite for '%s' to %s\n", name, savePath)
		return nil
	},
}

var serverPeerRename = &args.Command{
	Name: "rename",
	Help: "rename an existing peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
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

		// operands
		network := i.GetOperand("network")
		oldName := i.GetOperand("peer")
		newName := i.GetOperand("new-name")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		req := server.UpdatePeerRequest{
			Name: &newName,
		}
		_, err = srv.UpdatePeer(oldName, req)
		if err != nil {
			return fmt.Errorf("failed to rename peer: %w", err)
		}

		return nil
	},
}

var serverPeerEnable = &args.Command{
	Name: "enable",
	Help: "enable an existing peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to enable",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		req := server.UpdatePeerRequest{
			Enabled: boolPtr(true),
		}
		_, err = srv.UpdatePeer(peerName, req)
		if err != nil {
			return fmt.Errorf("failed to enable peer: %w", err)
		}

		return nil
	},
}

var serverPeerDisable = &args.Command{
	Name: "disable",
	Help: "disable an existing peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to disable",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		req := server.UpdatePeerRequest{
			Enabled: boolPtr(false),
		}
		_, err = srv.UpdatePeer(peerName, req)
		if err != nil {
			return fmt.Errorf("failed to disable peer: %w", err)
		}

		return nil
	},
}

var serverPeerDelete = &args.Command{
	Name: "delete",
	Help: "delete an existing peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to delete",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		err = srv.DeletePeer(peerName)
		if err != nil {
			return fmt.Errorf("failed to delete peer: %w", err)
		}

		return nil
	},
}

var serverPeerList = &args.Command{
	Name: "list",
	Help: "list a network's peers and their status",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to list peers from",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		jsonOut := i.GetFlag("json")

		// create server
		srv, err := newServerRead(i, network)
		if err != nil {
			return err
		}

		// execute
		statuses, err := srv.ListPeerStatuses()
		if err != nil {
			return fmt.Errorf("failed to list peers: %w", err)
		}

		// format output
		if jsonOut {
			return printJSON(statuses)
		}
		rows := make([][]string, 0, len(statuses))
		for _, status := range statuses {
			rows = append(rows, []string{
				status.Name,
				status.Cidr,
				yesNo(status.Admin),
				yesNo(status.Enabled),
				yesNo(status.Confirmed),
				truncateKey(status.PublicKey),
				formatLastSeen(status.LastEndpoint, status.LastSeen),
			})
		}
		printTable(
			[]string{"NAME", "CIDR", "ADMIN", "ENABLED", "CONFIRMED", "PUBLIC KEY", "LAST SEEN"},
			rows,
		)
		return nil
	},
}

var serverPeerVisible = &args.Command{
	Name: "visible",
	Help: "list the peers visible to a given peer",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of the cord network the server coordinates",
		},
		{
			Name: "peer",
			Help: "the name of the requesting peer",
		},
	},
	Options: []args.Option{jsonOption},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// options
		jsonOut := i.GetFlag("json")

		// create server
		srv, err := newServerRead(i, network)
		if err != nil {
			return err
		}

		// execute
		peers, err := srv.GetVisiblePeers(peerName)
		if err != nil {
			return fmt.Errorf("failed to get peers for '%s': %w", peerName, err)
		}

		// format output
		if jsonOut {
			return printJSON(peers)
		}
		rows := make([][]string, 0, len(peers))
		for _, peer := range peers {
			lastEndpoint, lastSeen := "", int64(0)
			if len(peer.Endpoints) > 0 {
				lastEndpoint = peer.Endpoints[0].Endpoint
				lastSeen = peer.Endpoints[0].Timestamp
			}
			rows = append(rows, []string{
				peer.Name,
				peer.Cidr,
				truncateKey(peer.PublicKey),
				formatLastSeen(lastEndpoint, lastSeen),
			})
		}
		printTable([]string{"NAME", "CIDR", "PUBLIC KEY", "LAST SEEN"}, rows)
		return nil
	},
}

func formatLastSeen(endpoint string, timestamp int64) string {
	if endpoint == "" {
		return "-"
	}
	return fmt.Sprintf("%s (%s)", endpoint, formatUnix(timestamp))
}

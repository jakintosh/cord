package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

const DEFAULT_INVITE_CIDR = "172.16.10.0/24"

var serverCmd = &args.Command{
	Name: "server",
	Help: "run and manage a cord coordination server",
	Subcommands: []*args.Command{
		serve,
		addNetwork,
		deleteNetwork,
		serverAddCidr,
		serverRenameCidr,
		serverDeleteCidr,
		serverAddPeer,
		serverRenamePeer,
		serverEnablePeer,
		serverDisablePeer,
		serverDeletePeer,
		serverGetPeers,
		serverAddAssociation,
		serverDeleteAssociation,
	},
}

var serve = &args.Command{
	Name: "serve",
	Help: "serve a cord coordination server",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "name of the cord network the server coordinates",
		},
	},
	Options: []args.Option{
		{
			Long: "no-routing",
			Type: args.OptionTypeFlag,
			Help: "tell cord not to handle routing",
		},
		{
			Long: "mtu",
			Type: args.OptionTypeParameter,
			Help: "MTU for the WireGuard interfaces (default 1420)",
		},
		{
			Long: "backend",
			Type: args.OptionTypeParameter,
			Help: "WireGuard backend ('auto', 'kernel' or 'userspace')",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)
		noRouting := i.GetFlag("no-routing")
		mtu := i.GetIntParameterOr("mtu", 1420)
		backendValue := i.GetParameterOr("backend", "auto")

		// parse
		backend, err := parseBackend(backendValue)
		if err != nil {
			return fmt.Errorf("failed to parse backend: %w", err)
		}

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		// build the runtime and the two API routers; mutations made
		// over the API poke the runtime for an immediate resync
		runtime, err := server.NewRuntime(ctx, noRouting, mtu, backend)
		if err != nil {
			return fmt.Errorf("failed to prepare server: %w", err)
		}

		apiServer, err := api.New(api.Options{
			Service:    ctx,
			OnMutation: runtime.Poke,
		})
		if err != nil {
			return fmt.Errorf("failed to create api: %w", err)
		}

		sigCtx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		err = runtime.Run(sigCtx, apiServer.Router(), apiServer.InviteRouter())
		if err != nil {
			return fmt.Errorf("failed to serve network '%s': %w", network, err)
		}

		return nil
	},
}

var addNetwork = &args.Command{
	Name: "add-network",
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
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)
		inviteCidrValue := i.GetParameterOr("invite-cidr", DEFAULT_INVITE_CIDR)

		// parse
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

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.CreateNetwork(server.CreateNetworkRequest{
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

var deleteNetwork = &args.Command{
	Name: "delete-network",
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

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeleteNetwork()
		if err != nil {
			return fmt.Errorf("failed to delete network: %w", err)
		}

		return nil
	},
}

var serverAddCidr = &args.Command{
	Name: "add-cidr",
	Help: "add a child CIDR to a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to add a CIDR to",
		},
		{
			Name: "name",
			Help: "name of the CIDR",
		},
		{
			Name: "cidr",
			Help: "address range in CIDR notation (i.e. 10.0.0.0/8)",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		// execute command
		req := server.CreateCidrRequest{
			Name: name,
			Cidr: cidr,
		}
		err = ctx.CreateCidr(req)
		if err != nil {
			return fmt.Errorf("failed to create cidr: %w", err)
		}

		return nil
	},
}

var serverRenameCidr = &args.Command{
	Name: "rename-cidr",
	Help: "rename an existing CIDR from a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
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

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		newName := i.GetOperand("new-name")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		// execute command
		req := server.UpdateCidrRequest{
			Name: newName,
		}
		err = ctx.UpdateCidr(cidr, req)
		if err != nil {
			return fmt.Errorf("failed to rename cidr: %w", err)
		}

		return nil
	},
}

var serverDeleteCidr = &args.Command{
	Name: "delete-cidr",
	Help: "delete an existing CIDR from a network",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "cidr",
			Help: "CIDR to delete",
		},
	},
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeleteCidr(cidr)
		if err != nil {
			return fmt.Errorf("failed to delete cidr: %w", err)
		}

		return nil
	},
}

var serverAddPeer = &args.Command{
	Name: "add-peer",
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
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)
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

		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.CreateInviteRequest{
			Name:       name,
			IP:         ip,
			Admin:      admin,
			Expiration: expiration,
		}
		invite, err := ctx.CreateInvite(req)
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

var serverRenamePeer = &args.Command{
	Name: "rename-peer",
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

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.UpdatePeerRequest{
			Name: &newName,
		}
		_, err = ctx.UpdatePeer(oldName, req)
		if err != nil {
			return fmt.Errorf("failed to rename peer: %w", err)
		}

		return nil
	},
}

var serverEnablePeer = &args.Command{
	Name: "enable-peer",
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

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.UpdatePeerRequest{
			Enabled: boolPtr(true),
		}
		_, err = ctx.UpdatePeer(peerName, req)
		if err != nil {
			return fmt.Errorf("failed to enable peer: %w", err)
		}

		return nil
	},
}

var serverDisablePeer = &args.Command{
	Name: "disable-peer",
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

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.UpdatePeerRequest{
			Enabled: boolPtr(false),
		}
		_, err = ctx.UpdatePeer(peerName, req)
		if err != nil {
			return fmt.Errorf("failed to disable peer: %w", err)
		}

		return nil
	},
}

var serverDeletePeer = &args.Command{
	Name: "delete-peer",
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

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeletePeer(peerName)
		if err != nil {
			return fmt.Errorf("failed to delete peer: %w", err)
		}

		return nil
	},
}

var serverGetPeers = &args.Command{
	Name: "get-peers",
	Help: "get peer list for a given peer",
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
	Handler: func(i *args.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		peers, err := ctx.GetVisiblePeers(peerName)
		if err != nil {
			return fmt.Errorf("failed to get peers for '%s': %w", peerName, err)
		}

		jsonBytes, err := json.MarshalIndent(peers, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal peers to json: %w", err)
		}

		fmt.Println(string(jsonBytes))

		return nil
	},
}

var serverAddAssociation = &args.Command{
	Name: "add-association",
	Help: "create an association between two CIDRs",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to add an association to",
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

		// operands
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.CreateAssociation(cidr1, cidr2)
		if err != nil {
			return fmt.Errorf("failed to create association: %w", err)
		}

		return nil
	},
}

var serverDeleteAssociation = &args.Command{
	Name: "delete-association",
	Help: "delete an association between two CIDRs",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network to delete an association from",
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

		// operands
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		// options
		configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)

		// create app context
		ctx, err := newServerContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeleteAssociation(cidr1, cidr2)
		if err != nil {
			return fmt.Errorf("failed to delete association: %w", err)
		}

		return nil
	},
}

func newServerContext(
	network string,
	configDir string,
	dataDir string,
) (*server.Context, error) {
	if err := ensureDirs(configDir, dataDir); err != nil {
		return nil, err
	}
	config := server.NewFsConfig(configDir)
	store, err := database.Init(network, dataDir, true)
	if err != nil {
		return nil, err
	}
	return server.NewContext(network, config, store)
}

func parseCidr(
	value string,
) (
	*net.IPNet,
	error,
) {
	_, cidr, err := net.ParseCIDR(value)
	if err != nil {
		err = fmt.Errorf("failed to parse cidr from operand '%s': %v", value, err)
	}
	return cidr, err
}

func parseIp(
	value string,
) (
	net.IP,
	error,
) {
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse ip from '%s'", value)
	}

	return utils.NormalizeIP(ip), nil
}

func parsePort(
	value string,
) (
	uint16,
	error,
) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("failed to parse port from '%s': %v", value, err)
	}
	return uint16(port), nil
}

func parseBackend(
	value string,
) (
	wg.BackendType,
	error,
) {
	switch value {
	case "auto":
		return wg.BackendAuto, nil
	case "kernel":
		return wg.BackendKernel, nil
	case "userspace":
		return wg.BackendUserspace, nil
	default:
		return wg.BackendAuto, fmt.Errorf("unexpected backend value: %s", value)
	}
}

func parseExpiration(
	value string,
) (
	time.Time,
	error,
) {
	if len(value) < 2 {
		return time.Time{}, fmt.Errorf("invalid expiration '%s'", value)
	}

	last := len(value) - 1
	number, err := strconv.ParseInt(value[0:last], 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	var multiplier int64
	switch value[last] {
	case 's':
		multiplier = 1
	case 'm':
		multiplier = 60
	case 'h':
		multiplier = 60 * 60
	case 'd':
		multiplier = 60 * 60 * 24
	case 'w':
		multiplier = 60 * 60 * 24 * 7
	default:
		return time.Time{}, fmt.Errorf("invalid expiration suffix '%c'", value[last])
	}

	return time.Now().Add(time.Duration(number*multiplier) * time.Second), nil
}

func getPwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("couldn't read pwd")
		os.Exit(1)
	}
	return dir
}

func boolPtr(b bool) *bool { return &b }

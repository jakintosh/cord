package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
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
		serverServe,
		serverNetworkCmd,
		serverCidrCmd,
		serverPeerCmd,
		serverAssociationCmd,
		serverInviteCmd,
	},
}

var serverServe = &args.Command{
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
		noRouting := i.GetFlag("no-routing")
		mtu := i.GetIntParameterOr("mtu", 1420)
		backendValue := i.GetParameterOr("backend", "auto")

		// parse
		backend, err := parseBackend(backendValue)
		if err != nil {
			return fmt.Errorf("failed to parse backend: %w", err)
		}

		// create server
		srv, err := newServer(i, network)
		if err != nil {
			return fmt.Errorf("failed to create server: %w", err)
		}

		// build the runtime and the two API routers; mutations made
		// over the API poke the runtime for an immediate resync
		runtime, err := server.NewRuntime(srv, noRouting, mtu, backend)
		if err != nil {
			return fmt.Errorf("failed to prepare server: %w", err)
		}

		apiServer, err := api.New(api.Options{
			Service:    srv,
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

func serverDirs(i *args.Input) (string, string) {
	configDir := i.GetParameterOr("config-dir", SERVER_DEFAULT_CFG)
	dataDir := i.GetParameterOr("data-dir", SERVER_DEFAULT_DATA)
	return configDir, dataDir
}

// newServer opens an existing network; it errors without creating any
// state if the network's database is missing.
func newServer(
	i *args.Input,
	network string,
) (
	*server.Server,
	error,
) {
	configDir, dataDir := serverDirs(i)
	return openServer(configDir, dataDir, network, true)
}

// newServerCreate opens a network for creation, creating directories
// and the database as needed.
func newServerCreate(
	i *args.Input,
	network string,
) (
	*server.Server,
	error,
) {
	configDir, dataDir := serverDirs(i)
	return openServer(configDir, dataDir, network, false)
}

func openServer(
	configDir string,
	dataDir string,
	network string,
	mustExist bool,
) (
	*server.Server,
	error,
) {
	dbOpts := database.Options{
		Name:      network,
		Dir:       dataDir,
		WAL:       true,
		MustExist: mustExist,
	}
	store, err := database.OpenServer(dbOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open network store: %w", err)
	}

	opts := server.Options{
		Network: network,
		Config:  server.NewFsConfig(configDir),
		Store:   store,
	}
	return server.New(opts)
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

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	adminserver "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

var serverCmd = &args.Command{
	Name: "server",
	Help: "manage the cord server daemon",
	Options: []args.Option{
		{
			Long: "socket-path",
			Type: args.OptionTypeParameter,
			Help: "path to the server daemon unix socket",
		},
		{
			Long: "json",
			Type: args.OptionTypeFlag,
			Help: "emit JSON instead of text",
		},
	},
	Subcommands: []*args.Command{
		serverDaemonCmd,
		serverStatusCmd,
		serverNetworkCmd,
		serverPeerCmd,
		serverCidrCmd,
		serverGroupCmd,
		serverAssociationCmd,
		serverRegistrationCmd,
	},
}

var serverDaemonCmd = &args.Command{
	Name: "daemon",
	Help: "run the cord server daemon",
	Options: []args.Option{
		{
			Long: "backend",
			Type: args.OptionTypeParameter,
			Help: "wireguard backend: auto, kernel, or userspace",
		},
		{
			Long: "debug",
			Type: args.OptionTypeFlag,
			Help: "enable verbose debug logging",
		},
		{
			Long: "socket-mode",
			Type: args.OptionTypeParameter,
			Help: "control socket permissions: 0600, 0660, or 0666",
		},
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		socketModeStr := i.GetParameterOr("socket-mode", "0660") // TODO: eventually this 0660 default should live some where more obvious
		backend := i.GetParameterOr("backend", "auto")
		debug := i.GetFlag("debug")

		socketMode, err := daemon.ParseSocketMode(socketModeStr)
		if err != nil {
			return err
		}

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		opts := server.Options{
			SocketPath: socketPath,
			SocketMode: socketMode,
			Backend:    backend,
			Version:    VersionInfo.Version,
			Debug:      debug,
		}
		return server.Serve(ctx, opts)
	},
}

var serverStatusCmd = &args.Command{
	Name: "status",
	Help: "show server resources and activity status",
	Handler: func(i *args.Input) error {
		client, err := serverClient(i)
		if err != nil {
			return err
		}

		result, err := client.Status(i.Context())
		if err != nil {
			return err
		}

		return cliApp(i).ServerStatus(result)
	},
}

// serverClient resolves the socket path and returns an admin API client
// for the server daemon.
func serverClient(
	i *args.Input,
) (
	*adminserver.Client,
	error,
) {
	return adminserver.New(serverSocket(i))
}

// serverSocket resolves the server daemon socket path from the CLI input,
// falling back to the server package's default.
func serverSocket(
	i *args.Input,
) string {
	return i.GetParameterOr("socket-path", adminserver.DefaultSocketPath)
}

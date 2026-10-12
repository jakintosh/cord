package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	adminclient "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
)

var clientCmd = &args.Command{
	Name: "client",
	Help: "manage the cord client daemon",
	Options: []args.Option{
		{
			Long: "socket-path",
			Type: args.OptionTypeParameter,
			Help: "path to the client daemon unix socket",
		},
		{
			Long: "json",
			Type: args.OptionTypeFlag,
			Help: "emit JSON instead of text",
		},
	},
	Subcommands: []*args.Command{
		clientDaemonCmd,
		clientStatusCmd,
		clientNetworkCmd,
		clientPeerCmd,
	},
}

var clientDaemonCmd = &args.Command{
	Name: "daemon",
	Help: "run the cord client daemon",
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
		socketPath := clientSocket(i)
		backend := i.GetParameterOr("backend", "auto")
		socketModeStr := i.GetParameterOr("socket-mode", "0660")

		socketMode, err := daemon.ParseSocketMode(socketModeStr)
		if err != nil {
			return err
		}

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		opts := client.Options{
			SocketPath: socketPath,
			SocketMode: socketMode,
			Backend:    backend,
			Version:    VersionInfo.Version,
			Debug:      i.GetFlag("debug"),
		}
		return client.Serve(ctx, opts)
	},
}

var clientStatusCmd = &args.Command{
	Name: "status",
	Help: "show client resources and activity status",
	Handler: func(i *args.Input) error {
		client, err := clientClient(i)
		if err != nil {
			return err
		}

		result, err := client.Status(i.Context())
		if err != nil {
			return err
		}

		return cliApp(i).ClientStatus(result)
	},
}

// clientClient resolves the socket path and returns an API client for the
// client daemon.
func clientClient(
	i *args.Input,
) (
	*adminclient.Client,
	error,
) {
	return adminclient.New(clientSocket(i))
}

// clientSocket resolves the client daemon socket path from the CLI input,
// falling back to the client package's default.
func clientSocket(
	i *args.Input,
) string {
	return i.GetParameterOr("socket-path", adminclient.DefaultSocketPath)
}

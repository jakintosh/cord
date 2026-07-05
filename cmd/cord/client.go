package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
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
		jsonOption,
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
	},
	Handler: func(i *args.Input) error {
		socketPath := clientSocket(i)
		backend := i.GetParameterOr("backend", "auto")

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		opts := client.Options{
			SocketPath: socketPath,
			Backend:    backend,
			Version:    VersionInfo.Version,
		}
		return client.Serve(ctx, opts)
	},
}

var clientStatusCmd = &args.Command{
	Name: "status",
	Help: "check if the cord client daemon is running",
	Handler: func(i *args.Input) error {
		socketPath := clientSocket(i)

		c := api.NewClient(socketPath)
		result, err := c.Status(context.Background())
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}
		fmt.Printf("client daemon ok (version %s)\n", result.Version)

		rows := make([][]string, len(result.Networks))
		for idx, n := range result.Networks {
			rows[idx] = []string{n.Name, n.State, strconv.FormatBool(n.Enabled), strconv.FormatBool(n.Connected)}
		}
		printTable([]string{"NAME", "STATE", "ENABLED", "CONNECTED"}, rows)
		return nil
	},
}

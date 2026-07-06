package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/server"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
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
		jsonOption,
	},
	Subcommands: []*args.Command{
		serverDaemonCmd,
		serverStatusCmd,
		serverNetworkCmd,
		serverPeerCmd,
		serverCidrCmd,
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
	},
	Handler: func(i *args.Input) error {
		socketPath := serverSocket(i)
		backend := i.GetParameterOr("backend", "auto")

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		opts := server.Options{
			SocketPath: socketPath,
			Backend:    backend,
			Version:    VersionInfo.Version,
		}
		return server.Serve(ctx, opts)
	},
}

var serverStatusCmd = &args.Command{
	Name: "status",
	Help: "check if the cord server daemon is running",
	Handler: func(i *args.Input) error {
		client, err := serverClient(i)
		if err != nil {
			return err
		}

		result, err := client.Status(i.Context())
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}
		printServerStatus(result)
		return nil
	},
}

// printServerStatus prints the ok/version line followed by a per-network
// status table.
func printServerStatus(
	s admin.StatusDTO,
) {
	fmt.Printf("server daemon ok (version %s)\n", s.Version)

	rows := make([][]string, len(s.Networks))
	for idx, n := range s.Networks {
		rows[idx] = []string{
			n.Name,
			strconv.FormatBool(n.Enabled),
			strconv.Itoa(n.PeerCount),
			strconv.Itoa(n.PendingRegistrationCount),
		}
	}
	printTable([]string{"NAME", "ENABLED", "PEERS", "PENDING REGISTRATIONS"}, rows)
}

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
			Debug:      i.GetFlag("debug"),
		}
		return client.Serve(ctx, opts)
	},
}

var clientStatusCmd = &args.Command{
	Name: "status",
	Help: "check if the cord client daemon is running",
	Handler: func(i *args.Input) error {
		client, err := clientClient(i)
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
		printClientStatus(result)
		return nil
	},
}

// printClientStatus prints the ok/version line followed by a table of
// installed networks and, when present, a table of in-progress installs.
func printClientStatus(
	s api.Status,
) {
	fmt.Printf("client daemon ok (version %s)\n", s.Version)

	if len(s.Networks) > 0 {
		rows := make([][]string, len(s.Networks))
		for idx, n := range s.Networks {
			rows[idx] = []string{
				n.Name,
				strconv.FormatBool(n.Enabled),
				strconv.FormatBool(n.Running),
				humanizeOptionalTime(n.Sync.LastRunAt),
				humanizeOptionalTime(n.Scan.LastRunAt),
				humanizeOptionalTime(n.Report.LastRunAt),
			}
		}
		printTable(
			[]string{"NETWORK", "ENABLED", "RUNNING", "LAST SYNC", "LAST SCAN", "LAST REPORT"},
			rows,
		)
	}

	if len(s.Installs) > 0 {
		rows := make([][]string, len(s.Installs))
		for idx, inst := range s.Installs {
			rows[idx] = []string{inst.Name, inst.State}
		}
		printTable([]string{"INSTALL", "STATE"}, rows)
	}
}

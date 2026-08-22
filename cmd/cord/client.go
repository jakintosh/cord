package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/client"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
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
		socketModeStr := i.GetParameterOr("socket-mode", "0666")

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

// printClientStatus prints daemon and network health followed, when
// present, by a table of in-progress installs.
func printClientStatus(
	s api.Status,
) {
	fmt.Printf(
		"client daemon ok (%s), managed networks %s\n\n",
		s.Version,
		s.Health,
	)

	if len(s.Networks) > 0 {
		rows := make([][]string, len(s.Networks))
		for idx, n := range s.Networks {
			rows[idx] = []string{
				n.Name,
				strconv.FormatBool(n.Enabled),
				strconv.FormatBool(n.Running),
				n.Health,
				humanizeOptionalTime(n.Sync.LastSuccessAt),
				humanizeOptionalTime(n.Scan.LastSuccessAt),
				humanizeOptionalTime(n.Report.LastSuccessAt),
				clientStatusDetail(n),
			}
		}
		printTable(
			[]string{
				"NETWORK",
				"ENABLED",
				"RUNNING",
				"HEALTH",
				"LAST SYNC",
				"LAST SCAN",
				"LAST REPORT",
				"DETAIL",
			},
			rows,
		)
	}

	if len(s.Installs) > 0 {
		rows := make([][]string, len(s.Installs))
		for idx, inst := range s.Installs {
			rows[idx] = []string{
				inst.Name,
				inst.State,
			}
		}
		printTable([]string{
			"INSTALL",
			"STATE",
		}, rows)
	}
}

func clientStatusDetail(
	status api.NetworkStatus,
) string {
	if status.Reason != "" {
		return status.Reason
	}
	activities := []struct {
		name   string
		status api.ActivityStatus
	}{
		{name: "reconcile", status: status.Reconcile},
		{name: "sync", status: status.Sync},
		{name: "scan", status: status.Scan},
		{name: "report", status: status.Report},
	}
	var details []string
	for _, activity := range activities {
		if activity.status.Error != "" {
			details = append(
				details,
				activity.name+": "+strings.ReplaceAll(
					activity.status.Error,
					"\n",
					"; ",
				),
			)
		}
	}
	if len(details) == 0 {
		return "-"
	}
	return strings.Join(details, "; ")
}

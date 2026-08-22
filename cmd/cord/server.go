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
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
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
		socketModeStr := i.GetParameterOr("socket-mode", "0666")
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

// printServerStatus prints daemon and per-network health.
func printServerStatus(
	s admin.Status,
) {
	fmt.Printf(
		"server daemon ok (version %s), managed networks %s\n",
		s.Version,
		s.Health,
	)

	rows := make([][]string, len(s.Networks))
	for idx, n := range s.Networks {
		rows[idx] = []string{
			n.Name,
			strconv.FormatBool(n.Enabled),
			strconv.FormatBool(n.Running),
			n.Health,
			humanizeOptionalTime(n.Reconcile.LastSuccessAt),
			serverStatusDetail(n),
		}
	}
	printTable([]string{
		"NAME",
		"ENABLED",
		"RUNNING",
		"HEALTH",
		"LAST RECONCILE",
		"DETAIL",
	}, rows)
}

func serverStatusDetail(
	status admin.NetworkStatus,
) string {
	if status.Reason != "" {
		return status.Reason
	}
	activities := []struct {
		name   string
		status admin.ActivityStatus
	}{
		{name: "reconcile", status: status.Reconcile},
		{name: "main api", status: status.MainAPI},
		{name: "invite api", status: status.InviteAPI},
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

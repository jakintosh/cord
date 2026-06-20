package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/control"
	"git.studiopollinator.com/pollinator/cord/internal/serverd"
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
	},
	Subcommands: []*args.Command{
		serverDaemonCmd,
		serverStatusCmd,
	},
}

var serverDaemonCmd = &args.Command{
	Name: "daemon",
	Help: "run the cord server daemon",
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		return serverd.Run(ctx, socketPath)
	},
}

var serverStatusCmd = &args.Command{
	Name: "status",
	Help: "check if the cord server daemon is running",
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", serverd.DefaultSocketPath)

		client := control.NewClient(socketPath)
		if err := client.Ping(context.Background()); err != nil {
			return err
		}

		fmt.Println("ok")
		return nil
	},
}

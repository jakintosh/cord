package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/clientd"
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
	},
	Subcommands: []*args.Command{
		clientDaemonCmd,
		clientStatusCmd,
	},
}

var clientDaemonCmd = &args.Command{
	Name: "daemon",
	Help: "run the cord client daemon",
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", clientd.DefaultSocketPath)

		ctx, cancel := signal.NotifyContext(
			context.Background(), os.Interrupt, syscall.SIGTERM,
		)
		defer cancel()

		return clientd.Run(ctx, socketPath)
	},
}

var clientStatusCmd = &args.Command{
	Name: "status",
	Help: "check if the cord client daemon is running",
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", clientd.DefaultSocketPath)

		client := clientd.NewClient(socketPath)
		if err := client.Status(context.Background()); err != nil {
			return err
		}

		fmt.Println("ok")
		return nil
	},
}

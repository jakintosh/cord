package main

import (
	"context"
	"fmt"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/control"
)

var statusCmd = &args.Command{
	Name: "status",
	Help: "check if the cord daemon is running",
	Handler: func(i *args.Input) error {
		socketPath := i.GetParameterOr("socket-path", DEFAULT_SOCK)

		client := control.NewClient(socketPath)
		if err := client.Ping(context.Background()); err != nil {
			return err
		}

		fmt.Println("ok")
		return nil
	},
}

package serverd

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/internal/control"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

const DefaultSocketPath = "/tmp/cord-server.sock"

func Run(
	ctx context.Context,
	socketPath string,
) error {
	d, err := daemon.New(socketPath, control.NewHandler())
	if err != nil {
		return err
	}
	return d.Run(ctx)
}

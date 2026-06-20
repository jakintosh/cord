package clientd

import (
	"context"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

const DefaultSocketPath = "/tmp/cord-client.sock"

func Run(
	ctx context.Context,
	socketPath string,
) error {
	d, err := daemon.New(socketPath, newHandler())
	if err != nil {
		return err
	}
	return d.Run(ctx)
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", handleStatus)
	return mux
}

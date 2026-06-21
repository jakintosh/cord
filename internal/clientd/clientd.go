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

	mux.HandleFunc("GET /networks", handleNetworkList)
	mux.HandleFunc("GET /networks/{name}", handleNetworkShow)
	mux.HandleFunc("POST /networks", handleNetworkInstall)
	mux.HandleFunc("DELETE /networks/{name}", handleNetworkUninstall)

	mux.HandleFunc("POST /networks/{name}/enable", handleNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", handleNetworkDisable)
	mux.HandleFunc("POST /networks/{name}/up", handleNetworkUp)
	mux.HandleFunc("POST /networks/{name}/down", handleNetworkDown)
	mux.HandleFunc("POST /networks/{name}/fetch", handleNetworkFetch)

	return mux
}

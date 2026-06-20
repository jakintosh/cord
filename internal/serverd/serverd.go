package serverd

import (
	"context"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

const DefaultSocketPath = "/tmp/cord-server.sock"

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
	mux.HandleFunc("POST /networks", handleNetworkAdd)
	mux.HandleFunc("DELETE /networks/{name}", handleNetworkDelete)

	mux.HandleFunc("GET /networks/{name}/peers", handlePeerList)
	mux.HandleFunc("POST /networks/{name}/peers", handlePeerAdd)
	mux.HandleFunc("PATCH /networks/{name}/peers/{peer}", handlePeerRename)
	mux.HandleFunc("DELETE /networks/{name}/peers/{peer}", handlePeerDelete)
	mux.HandleFunc("POST /networks/{name}/peers/{peer}/enable", handlePeerEnable)
	mux.HandleFunc("POST /networks/{name}/peers/{peer}/disable", handlePeerDisable)
	mux.HandleFunc("GET /networks/{name}/peers/visible", handlePeerVisible)

	mux.HandleFunc("GET /networks/{name}/cidrs", handleCidrList)
	mux.HandleFunc("POST /networks/{name}/cidrs", handleCidrAdd)
	mux.HandleFunc("PATCH /networks/{name}/cidrs/{cidr}", handleCidrRename)
	mux.HandleFunc("DELETE /networks/{name}/cidrs/{cidr}", handleCidrDelete)

	mux.HandleFunc("GET /networks/{name}/associations", handleAssociationList)
	mux.HandleFunc("POST /networks/{name}/associations", handleAssociationAdd)
	mux.HandleFunc("POST /networks/{name}/associations/delete", handleAssociationDelete)

	mux.HandleFunc("GET /networks/{name}/invites", handleInviteList)

	return mux
}

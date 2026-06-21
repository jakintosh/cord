package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

const DefaultSocketPath = "/tmp/cord-server.sock"

type Options struct {
	Service *service.Service
	Logger  *log.Logger
}

type API struct {
	service *service.Service
	log     *log.Logger
}

func New(
	opts Options,
) (
	*API,
	error,
) {
	return &API{
		service: opts.Service,
		log:     opts.Logger,
	}, nil
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", a.handleStatus)

	mux.HandleFunc("GET /networks", a.handleNetworkList)
	mux.HandleFunc("GET /networks/{name}", a.handleNetworkShow)
	mux.HandleFunc("POST /networks", a.handleNetworkAdd)
	mux.HandleFunc("DELETE /networks/{name}", a.handleNetworkDelete)

	mux.HandleFunc("GET /networks/{name}/peers", a.handlePeerList)
	mux.HandleFunc("POST /networks/{name}/peers", a.handlePeerAdd)
	mux.HandleFunc("PATCH /networks/{name}/peers/{peer}", a.handlePeerRename)
	mux.HandleFunc("DELETE /networks/{name}/peers/{peer}", a.handlePeerDelete)
	mux.HandleFunc("POST /networks/{name}/peers/{peer}/enable", a.handlePeerEnable)
	mux.HandleFunc("POST /networks/{name}/peers/{peer}/disable", a.handlePeerDisable)
	mux.HandleFunc("GET /networks/{name}/peers/visible", a.handlePeerVisible)

	mux.HandleFunc("GET /networks/{name}/cidrs", a.handleCidrList)
	mux.HandleFunc("POST /networks/{name}/cidrs", a.handleCidrAdd)
	mux.HandleFunc("PATCH /networks/{name}/cidrs/{cidr}", a.handleCidrRename)
	mux.HandleFunc("DELETE /networks/{name}/cidrs/{cidr}", a.handleCidrDelete)

	mux.HandleFunc("GET /networks/{name}/associations", a.handleAssociationList)
	mux.HandleFunc("POST /networks/{name}/associations", a.handleAssociationAdd)
	mux.HandleFunc("POST /networks/{name}/associations/delete", a.handleAssociationDelete)

	mux.HandleFunc("GET /networks/{name}/invites", a.handleInviteList)

	return mux
}

func Run(
	ctx context.Context,
	socketPath string,
) error {
	svcOpts := service.Options{
		Store:  nil,
		WG:     nil,
		Clock:  time.Now,
		Logger: log.Default(),
	}
	svc, _ := service.New(svcOpts)

	apiOpts := Options{
		Service: svc,
	}
	api, _ := New(apiOpts)
	d, err := daemon.New(socketPath, api.Router())
	if err != nil {
		return err
	}
	return d.Run(ctx)
}

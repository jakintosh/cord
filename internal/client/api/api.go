package api

import (
	"log/slog"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

type Options struct {
	Service *service.Service
	Logger  *slog.Logger
	Version string
}

type API struct {
	service *service.Service
	log     *slog.Logger
	version string
}

func New(
	opts Options,
) (
	*API,
	error,
) {
	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}
	return &API{
		service: opts.Service,
		log:     log,
		version: opts.Version,
	}, nil
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", a.handleGetStatus)

	mux.HandleFunc("GET /networks", a.handleListNetworks)
	mux.HandleFunc("GET /networks/{name}", a.handleGetNetwork)
	mux.HandleFunc("POST /networks", a.handlePostNetwork)
	mux.HandleFunc("PATCH /networks/{name}", a.handlePatchNetwork)
	mux.HandleFunc("DELETE /networks/{name}", a.handleDeleteNetwork)
	mux.HandleFunc("POST /networks/{name}/redeem", a.handlePostNetworkRedeem)
	mux.HandleFunc("POST /networks/{name}/confirm", a.handlePostNetworkConfirm)
	mux.HandleFunc("POST /networks/{name}/enable", a.handlePostNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", a.handlePostNetworkDisable)
	mux.HandleFunc("POST /networks/{name}/sync", a.handPostNetworkSync)

	mux.HandleFunc("GET /networks/{name}/peers", a.handleListPeers)

	return logging.Middleware(a.log, mux)
}

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

	mux.HandleFunc("GET /status", a.handleStatus)

	mux.HandleFunc("GET /networks", a.handleNetworkList)
	mux.HandleFunc("GET /networks/{name}", a.handleNetworkShow)
	mux.HandleFunc("POST /networks", a.handleNetworkInstall)
	mux.HandleFunc("DELETE /networks/{name}", a.handleNetworkUninstall)
	mux.HandleFunc("GET /networks/{name}/peers", a.handlePeerList)

	mux.HandleFunc("POST /networks/{name}/redeem", a.handleNetworkRedeem)
	mux.HandleFunc("POST /networks/{name}/confirm", a.handleNetworkConfirm)
	mux.HandleFunc("POST /networks/{name}/enable", a.handleNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", a.handleNetworkDisable)
	mux.HandleFunc("POST /networks/{name}/sync", a.handleNetworkSync)
	mux.HandleFunc("POST /networks/{name}/listen-port", a.handleNetworkListenPort)

	return logging.Middleware(a.log, mux)
}

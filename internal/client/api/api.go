package api

import (
	"log"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

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
	mux.HandleFunc("POST /networks", a.handleNetworkInstall)
	mux.HandleFunc("DELETE /networks/{name}", a.handleNetworkUninstall)

	mux.HandleFunc("POST /networks/{name}/enable", a.handleNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", a.handleNetworkDisable)
	mux.HandleFunc("POST /networks/{name}/fetch", a.handleNetworkFetch)

	return mux
}

package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

type Options struct {
	Service *service.Service
	Runtime *runtime.Runtime
	Logger  *slog.Logger
	Version string
}

type API struct {
	service *service.Service
	runtime *runtime.Runtime
	log     *slog.Logger
	version string
}

func New(
	opts Options,
) (
	*API,
	error,
) {
	if opts.Service == nil {
		return nil, fmt.Errorf("client: service required")
	}
	if opts.Runtime == nil {
		return nil, fmt.Errorf("client: runtime required")
	}

	log := opts.Logger
	if log == nil {
		log = logging.Discard()
	}
	return &API{
		service: opts.Service,
		runtime: opts.Runtime,
		log:     log,
		version: opts.Version,
	}, nil
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", a.handleGetStatus)

	mux.HandleFunc("GET /networks", a.handleListNetworks)
	mux.HandleFunc("GET /networks/{name}", a.handleGetNetwork)
	mux.HandleFunc("GET /networks/{name}/topology", a.handleGetNetworkTopology)
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

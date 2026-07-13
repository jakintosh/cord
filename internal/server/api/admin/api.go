package admin

import (
	"log/slog"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
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
	mux.HandleFunc("POST /networks/{name}/enable", a.handlePostNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", a.handlePostNetworkDisable)
	mux.HandleFunc("DELETE /networks/{name}", a.handleDeleteNetwork)

	mux.HandleFunc("GET /networks/{name}/peers", a.handleListPeers)

	mux.HandleFunc("PATCH /networks/{name}/peers/{peer}", a.handlePatchPeer)
	mux.HandleFunc("DELETE /networks/{name}/peers/{peer}", a.handleDeletePeer)

	mux.HandleFunc("GET /networks/{name}/cidrs", a.handleListCidrs)
	mux.HandleFunc("POST /networks/{name}/cidrs", a.handlePostCidr)
	mux.HandleFunc("PATCH /networks/{name}/cidrs/{cidr}", a.handlePatchCidr)
	mux.HandleFunc("DELETE /networks/{name}/cidrs/{cidr}", a.handleDeleteCidr)

	mux.HandleFunc("GET /networks/{name}/associations", a.handleListAssociations)
	mux.HandleFunc("POST /networks/{name}/associations", a.handlePostAssociation)
	mux.HandleFunc("POST /networks/{name}/associations/delete", a.handlePostAssociationDelete)

	mux.HandleFunc("GET /networks/{name}/registrations", a.handleListRegistrations)
	mux.HandleFunc("POST /networks/{name}/registrations", a.handlePostRegistration)
	mux.HandleFunc("DELETE /networks/{name}/registrations/{registration}", a.handleDeleteRegistration)

	return logging.Middleware(a.log, mux)
}

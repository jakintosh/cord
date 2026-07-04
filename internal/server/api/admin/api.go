package admin

import (
	"log"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
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
	mux.HandleFunc("POST /networks", a.handleNetworkAdd)
	mux.HandleFunc("DELETE /networks/{name}", a.handleNetworkDelete)
	mux.HandleFunc("POST /networks/{name}/enable", a.handleNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", a.handleNetworkDisable)

	mux.HandleFunc("GET /networks/{name}/peers", a.handlePeerList)
	mux.HandleFunc("POST /networks/{name}/registrations", a.handleInviteCreate)
	mux.HandleFunc("PATCH /networks/{name}/peers/{peer}", a.handlePeerRename)
	mux.HandleFunc("DELETE /networks/{name}/peers/{peer}", a.handlePeerDelete)
	mux.HandleFunc("POST /networks/{name}/peers/{peer}/enable", a.handlePeerEnable)
	mux.HandleFunc("POST /networks/{name}/peers/{peer}/disable", a.handlePeerDisable)

	mux.HandleFunc("GET /networks/{name}/cidrs", a.handleCidrList)
	mux.HandleFunc("POST /networks/{name}/cidrs", a.handleCidrAdd)
	mux.HandleFunc("PATCH /networks/{name}/cidrs/{cidr}", a.handleCidrRename)
	mux.HandleFunc("DELETE /networks/{name}/cidrs/{cidr}", a.handleCidrDelete)

	mux.HandleFunc("GET /networks/{name}/associations", a.handleAssociationList)
	mux.HandleFunc("POST /networks/{name}/associations", a.handleAssociationAdd)
	mux.HandleFunc("POST /networks/{name}/associations/delete", a.handleAssociationDelete)

	mux.HandleFunc("GET /networks/{name}/registrations", a.handleRegistrationList)

	return mux
}

package admin

import (
	"fmt"
	"log/slog"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

type ActivityStatus = server.ActivityStatus
type Association = server.Association
type Cidr = server.Cidr
type CidrGroupRequest = server.CidrGroupRequest
type CreateAssociationRequest = server.CreateAssociationRequest
type CreateCidrRequest = server.CreateCidrRequest
type CreateGroupRequest = server.CreateGroupRequest
type CreateNetworkRequest = server.CreateNetworkRequest
type CreateRegistrationRequest = server.CreateRegistrationRequest
type Group = server.Group
type Network = server.Network
type NetworkStatus = server.NetworkStatus
type NetworkTopology = server.NetworkTopology
type Peer = server.Peer
type Registration = server.Registration
type RegistrationGroupRequest = server.RegistrationGroupRequest
type Status = server.Status
type TopologyAssociation = server.TopologyAssociation
type TopologyNode = server.TopologyNode
type UpdateCidrRequest = server.UpdateCidrRequest
type UpdatePeerRequest = server.UpdatePeerRequest

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
		return nil, fmt.Errorf("server: service required")
	}

	if opts.Runtime == nil {
		return nil, fmt.Errorf("server: runtime required")
	}

	logger := opts.Logger
	if logger == nil {
		logger = logging.Discard()
	}

	return &API{
		service: opts.Service,
		runtime: opts.Runtime,
		log:     logger,
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
	mux.HandleFunc("POST /networks/{name}/enable", a.handlePostNetworkEnable)
	mux.HandleFunc("POST /networks/{name}/disable", a.handlePostNetworkDisable)
	mux.HandleFunc("DELETE /networks/{name}", a.handleDeleteNetwork)

	mux.HandleFunc(
		"GET /networks/{name}/registrations",
		a.handleListRegistrations,
	)
	mux.HandleFunc(
		"POST /networks/{name}/registrations",
		a.handlePostRegistration,
	)
	mux.HandleFunc(
		"DELETE /networks/{name}/registrations/{registration}",
		a.handleDeleteRegistration,
	)
	mux.HandleFunc(
		"GET /networks/{name}/registrations/{registration}/groups",
		a.handleListRegistrationGroups,
	)
	mux.HandleFunc(
		"POST /networks/{name}/registrations/{registration}/groups",
		a.handlePostRegistrationGroup,
	)
	mux.HandleFunc(
		"DELETE /networks/{name}/registrations/{registration}/groups/{group}",
		a.handleDeleteRegistrationGroup,
	)

	mux.HandleFunc("GET /networks/{name}/cidrs", a.handleListCidrs)
	mux.HandleFunc("POST /networks/{name}/cidrs", a.handlePostCidr)
	mux.HandleFunc("PATCH /networks/{name}/cidrs/{cidr}", a.handlePatchCidr)
	mux.HandleFunc("DELETE /networks/{name}/cidrs/{cidr}", a.handleDeleteCidr)
	mux.HandleFunc("GET /networks/{name}/cidrs/{cidr}/groups", a.handleListCidrGroups)
	mux.HandleFunc("POST /networks/{name}/cidrs/{cidr}/groups", a.handlePostCidrGroup)
	mux.HandleFunc("DELETE /networks/{name}/cidrs/{cidr}/groups/{group}", a.handleDeleteCidrGroup)

	mux.HandleFunc("GET /networks/{name}/peers", a.handleListPeers)
	mux.HandleFunc("PATCH /networks/{name}/peers/{peer}", a.handlePatchPeer)
	mux.HandleFunc("DELETE /networks/{name}/peers/{peer}", a.handleDeletePeer)

	mux.HandleFunc("GET /networks/{name}/groups", a.handleListGroups)
	mux.HandleFunc("POST /networks/{name}/groups", a.handlePostGroup)
	mux.HandleFunc("DELETE /networks/{name}/groups/{group}", a.handleDeleteGroup)

	mux.HandleFunc("GET /networks/{name}/associations", a.handleListAssociations)
	mux.HandleFunc("POST /networks/{name}/associations", a.handlePostAssociation)
	mux.HandleFunc("DELETE /networks/{name}/associations/{group1}/{group2}", a.handleDeleteAssociation)

	return logging.Middleware(a.log, mux)
}

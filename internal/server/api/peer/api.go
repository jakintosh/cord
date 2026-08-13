package peer

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type API struct {
	service *service.Service
	network string
	log     *slog.Logger
}

func New(
	service *service.Service,
	network string,
	log *slog.Logger,
) *API {
	if log == nil {
		log = logging.Discard()
	}
	return &API{
		service: service,
		network: network,
		log:     log,
	}
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /snapshot", identity.Require(a.log, a.lookupPeer, a.handleVisibleSnapshot))
	mux.HandleFunc("POST /endpoints", identity.Require(a.log, a.lookupPeer, a.handleReportEndpoints))
	mux.HandleFunc("POST /confirm", identity.Require(a.log, a.lookupProvisional, a.handleConfirmPeer))
	return logging.Middleware(a.log, mux)
}

// lookupPeer looks up a confirmed, enabled peer by source IP.
func (a *API) lookupPeer(
	ip net.IP,
) (
	*identity.Peer,
	error,
) {
	p, err := a.service.ResolvePeerIdentity(a.network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve peer identity: %w", err)
	}
	return &identity.Peer{
		PublicKey: p.PublicKey,
		Name:      p.Name,
	}, nil
}

// lookupProvisional looks up an unconfirmed, enabled peer by source IP.
func (a *API) lookupProvisional(
	ip net.IP,
) (
	*identity.Peer,
	error,
) {
	p, err := a.service.ResolveProvisionalIdentity(a.network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve provisional identity: %w", err)
	}
	return &identity.Peer{
		PublicKey: p.PublicKey,
		Name:      p.Name,
	}, nil
}

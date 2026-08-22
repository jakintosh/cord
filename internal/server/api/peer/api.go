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

// API serves the peer-facing HTTP surface over a network's main plane.
// One API serves every network: the network name is bound per router.
type API struct {
	service *service.Service
	log     *slog.Logger
}

func New(
	service *service.Service,
	log *slog.Logger,
) *API {
	if log == nil {
		log = logging.Discard()
	}
	return &API{
		service: service,
		log:     log,
	}
}

// Router returns the handler for one network's main plane.
func (a *API) Router(
	network string,
) http.Handler {
	log := a.log.With("network", network)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /snapshot", identity.Require(
		log, a.lookupPeer(network), a.handleVisibleSnapshot(network),
	))
	mux.HandleFunc("POST /endpoints", identity.Require(
		log, a.lookupPeer(network), a.handleReportEndpoints(network),
	))
	mux.HandleFunc("POST /confirm", identity.Require(
		log, a.lookupProvisional(network), a.handleConfirmPeer(network),
	))
	return logging.Middleware(log, mux)
}

// lookupPeer looks up a confirmed, enabled peer by source IP.
func (a *API) lookupPeer(
	network string,
) identity.LookupFunc {
	return func(ip net.IP) (*identity.Peer, error) {
		p, err := a.service.ResolvePeerIdentity(network, ip)
		if err != nil {
			return nil, fmt.Errorf("resolve peer identity: %w", err)
		}
		return &identity.Peer{
			PublicKey: p.PublicKey,
			Name:      p.Name,
		}, nil
	}
}

// lookupProvisional looks up an unconfirmed, enabled peer by source IP.
func (a *API) lookupProvisional(
	network string,
) identity.LookupFunc {
	return func(ip net.IP) (*identity.Peer, error) {
		p, err := a.service.ResolveProvisionalIdentity(network, ip)
		if err != nil {
			return nil, fmt.Errorf("resolve provisional identity: %w", err)
		}
		return &identity.Peer{
			PublicKey: p.PublicKey,
			Name:      p.Name,
		}, nil
	}
}

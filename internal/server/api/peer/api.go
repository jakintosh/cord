package peer

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type API struct {
	service  *service.Service
	network  string
	resolver identity.Resolver
	log      *log.Logger
}

func New(service *service.Service, network string, log *log.Logger) *API {
	a := &API{
		service: service,
		network: network,
		log:     log,
	}
	a.resolver = a
	return a
}

func (a *API) SetResolver(resolver identity.Resolver) {
	a.resolver = resolver
}

// ResolveIdentity looks up a confirmed, enabled peer by source IP.
// Satisfies identity.Resolver. Used for /peers and /endpoints.
func (a *API) ResolveIdentity(ip net.IP) (*identity.Peer, error) {
	p, err := a.service.ResolvePeerIdentity(a.network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve peer identity: %w", err)
	}
	return &identity.Peer{PublicKey: p.PublicKey, Name: p.Name}, nil
}

// ResolveProvisionalIdentity looks up an unconfirmed, enabled peer by
// source IP. Satisfies identity.ProvisionalResolver. Used for /confirm.
func (a *API) ResolveProvisionalIdentity(ip net.IP) (*identity.Peer, error) {
	p, err := a.service.ResolveProvisionalIdentity(a.network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve provisional identity: %w", err)
	}
	return &identity.Peer{PublicKey: p.PublicKey, Name: p.Name}, nil
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /peers", a.handleVisiblePeers)
	mux.HandleFunc("POST /endpoints", a.handleReportEndpoints)
	mux.HandleFunc("POST /confirm", a.handleConfirmPeer)
	return mux
}

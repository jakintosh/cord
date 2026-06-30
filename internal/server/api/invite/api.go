package invite

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

func New(
	service *service.Service,
	network string,
	log *log.Logger,
) *API {
	a := &API{
		service: service,
		network: network,
		log:     log,
	}
	a.resolver = a
	return a
}

func (a *API) SetResolver(
	resolver identity.Resolver,
) {
	a.resolver = resolver
}

// ResolveIdentity looks up an unredeemed, unexpired invite by temporary
// IP within the API's network. Satisfies identity.Resolver.
func (a *API) ResolveIdentity(ip net.IP) (*identity.Peer, error) {
	inv, err := a.service.ResolveInviteIdentity(a.network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve invite identity: %w", err)
	}
	return &identity.Peer{PublicKey: inv.InvitePubKey, Name: inv.Name}, nil
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /redeem", a.handleRedeemInvite)
	return mux
}

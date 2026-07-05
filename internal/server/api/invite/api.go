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
	service *service.Service
	network string
	log     *log.Logger
}

func New(
	service *service.Service,
	network string,
	log *log.Logger,
) *API {
	return &API{
		service: service,
		network: network,
		log:     log,
	}
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /redeem", identity.Require(a.lookupRegistration, a.handleRedeemInvite))
	return mux
}

// lookupRegistration looks up an unredeemed, unexpired registration by
// temporary IP within the API's network.
func (a *API) lookupRegistration(
	ip net.IP,
) (
	*identity.Peer,
	error,
) {
	reg, err := a.service.ResolveRegistrationIdentity(a.network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve registration identity: %w", err)
	}
	return &identity.Peer{PublicKey: reg.InvitePublicKey, Name: reg.Name}, nil
}

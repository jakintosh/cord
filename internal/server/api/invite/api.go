package invite

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
	mux.HandleFunc("POST /redeem", identity.Require(a.log, a.lookupRegistration, a.handleRedeemInvite))
	return logging.Middleware(a.log, mux)
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

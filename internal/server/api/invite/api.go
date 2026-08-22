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

// API serves the invite-facing HTTP surface over a network's invite
// plane. One API serves every network: the network name is bound per
// router.
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

// Router returns the handler for one network's invite plane.
func (a *API) Router(
	network string,
) http.Handler {
	log := a.log.With("network", network)

	mux := http.NewServeMux()
	mux.HandleFunc(
		"POST /redeem",
		identity.Require(
			log,
			a.lookupRegistration(network),
			a.handleRedeemInvite(network),
		),
	)
	return logging.Middleware(log, mux)
}

// lookupRegistration looks up an unredeemed, unexpired registration by
// temporary IP within the network.
func (a *API) lookupRegistration(
	network string,
) identity.LookupFunc {
	return func(ip net.IP) (*identity.Peer, error) {
		reg, err := a.service.ResolveRegistrationIdentity(network, ip)
		if err != nil {
			return nil, fmt.Errorf("resolve registration identity: %w", err)
		}
		return &identity.Peer{PublicKey: reg.InvitePublicKey, Name: reg.Name}, nil
	}
}

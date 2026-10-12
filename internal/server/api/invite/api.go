package invite

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
)

// API serves the invite-facing HTTP surface over a network's invite
// plane. One API serves every network: the network name is bound per
// router.
type API struct {
	runtime *runtime.Runtime
	log     *slog.Logger
}

func New(
	runtime *runtime.Runtime,
	log *slog.Logger,
) *API {
	if log == nil {
		log = logging.Discard()
	}
	return &API{
		runtime: runtime,
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
			a.lookupRegistration(network),
			a.handleRedeemInvite(network),
			log,
		),
	)
	return logging.Middleware(log, mux)
}

// lookupRegistration looks up an unconfirmed, unexpired registration by
// temporary IP within the network.
func (a *API) lookupRegistration(
	network string,
) identity.LookupFunc {
	return func(
		ctx context.Context,
		ip net.IP,
	) (
		*identity.Peer,
		error,
	) {
		reg, err := a.runtime.ResolveRegistrationIdentity(ctx, network, ip)
		if err != nil {
			return nil, fmt.Errorf("resolve registration identity: %w", err)
		}

		return &identity.Peer{
			PublicKey: reg.InvitePublicKey,
			Name:      reg.Name,
		}, nil
	}
}

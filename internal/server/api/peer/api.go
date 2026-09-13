package peer

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

// API serves the peer-facing HTTP surface over a network's main plane.
// One API serves every network: the network name is bound per router.
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

// Router returns the handler for one network's main plane.
func (a *API) Router(
	network string,
) http.Handler {
	log := a.log.With("network", network)

	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /snapshot",
		identity.Require(
			a.lookupPeer(network),
			a.handleVisibleSnapshot(network),
			log,
		),
	)
	mux.HandleFunc(
		"POST /endpoints",
		identity.Require(
			a.lookupPeer(network),
			a.handleReportEndpoints(network),
			log,
		),
	)
	mux.HandleFunc(
		"POST /confirm",
		identity.Require(
			a.lookupProvisional(network),
			a.handleConfirmPeer(network),
			log,
		),
	)
	return logging.Middleware(log, mux)
}

// lookupPeer looks up a confirmed, enabled peer by source IP.
func (a *API) lookupPeer(
	network string,
) identity.LookupFunc {
	return func(ctx context.Context, ip net.IP) (*identity.Peer, error) {
		p, err := a.runtime.ResolvePeerIdentity(ctx, network, ip)
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
	return func(
		ctx context.Context,
		ip net.IP,
	) (
		*identity.Peer,
		error,
	) {
		p, err := a.runtime.ResolveProvisionalIdentity(ctx, network, ip)
		if err != nil {
			return nil, fmt.Errorf("resolve provisional identity: %w", err)
		}

		return &identity.Peer{
			PublicKey: p.PublicKey,
			Name:      p.Name,
		}, nil
	}
}

package api

import (
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// Options carries the dependencies for constructing an API.
type Options struct {
	// Service is the cord network service the API exposes.
	Service *server.Context

	// OnMutation, when set, is called after every successful state
	// change so the serving runtime can resync WireGuard interfaces
	// immediately. Optional.
	OnMutation func()
}

// API is the HTTP contract boundary for a cord server. It exposes two
// route trees: the full API served on the main network, and a
// redemption-only API served on the invite network (see ADR-001).
type API struct {
	service    *server.Context
	onMutation func()
}

func New(
	opts Options,
) (
	*API,
	error,
) {
	if opts.Service == nil {
		return nil, errors.New("api: service required")
	}

	return &API{
		service:    opts.Service,
		onMutation: opts.OnMutation,
	}, nil
}

// Router builds the full API route tree, served on the main network's
// internal address.
func (a *API) Router() http.Handler {
	v1 := http.NewServeMux()
	v1.HandleFunc("GET /peers", a.withMainAuth(a.handleListPeers))
	v1.HandleFunc("POST /endpoint", a.withMainAuth(a.handleReportEndpoints))
	v1.HandleFunc("POST /invite/confirm", a.handleConfirmInvite)
	wire.Subrouter(v1, "/admin", a.withAdminAuth(a.buildAdminRouter()))

	root := http.NewServeMux()
	wire.Subrouter(root, "/api/v1", v1)
	return root
}

// InviteRouter builds the redemption-only route tree, served on the
// invite network's internal address. The invite network must not
// expose anything else.
func (a *API) InviteRouter() http.Handler {
	v1 := http.NewServeMux()
	v1.HandleFunc("POST /invite/redeem", a.withInviteAuth(a.handleRedeemInvite))

	root := http.NewServeMux()
	wire.Subrouter(root, "/api/v1", v1)
	return root
}

// buildAdminRouter composes the admin route groups. Admin auth wraps
// the whole subtree at the mount point in Router().
func (a *API) buildAdminRouter() http.Handler {
	mux := http.NewServeMux()
	a.addAdminPeerRoutes(mux)
	a.addAdminCidrRoutes(mux)
	a.addAdminAssociationRoutes(mux)
	return mux
}

// mutated notifies the serving runtime of a state change.
func (a *API) mutated() {
	if a.onMutation != nil {
		a.onMutation()
	}
}

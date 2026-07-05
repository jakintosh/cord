package identity

import (
	"context"
	"net"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

type Peer struct {
	PublicKey string
	Name      string
}

type contextKey struct{}

// LookupFunc resolves a WireGuard-validated source IP to a caller.
type LookupFunc func(ip net.IP) (*Peer, error)

// Require wraps a handler with identity resolution: parse the source IP
// from r.RemoteAddr, call lookup, write 403 "identity unknown" on any
// failure, otherwise stash the *Peer in the request context and call next.
func Require(
	lookup LookupFunc,
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			wire.WriteError(w, http.StatusForbidden, "identity unknown")
			return
		}

		ip := net.ParseIP(host)
		if ip == nil {
			wire.WriteError(w, http.StatusForbidden, "identity unknown")
			return
		}

		peer, err := lookup(ip)
		if err != nil {
			wire.WriteError(w, http.StatusForbidden, "identity unknown")
			return
		}

		ctx := context.WithValue(r.Context(), contextKey{}, peer)
		next(w, r.WithContext(ctx))
	}
}

// Caller returns the *Peer stored by Require.
func Caller(
	ctx context.Context,
) *Peer {
	p, _ := ctx.Value(contextKey{}).(*Peer)
	return p
}

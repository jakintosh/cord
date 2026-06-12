package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// ctxKey is a private type for request context values set by middleware.
type ctxKey int

const (
	peerNameKey ctxKey = iota
	inviteKey
)

// peerName retrieves the authenticated peer name set by withMainAuth
// or withAdminAuth.
func peerName(r *http.Request) (string, bool) {
	name, ok := r.Context().Value(peerNameKey).(string)
	return name, ok
}

// authedInvite retrieves the invite record set by withInviteAuth.
func authedInvite(r *http.Request) (*server.ServerInvite, bool) {
	invite, ok := r.Context().Value(inviteKey).(*server.ServerInvite)
	return invite, ok
}

// withMainAuth authenticates a confirmed, enabled peer by its
// WireGuard-validated source IP on the main network.
func (a *API) withMainAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		peer, ok := a.peerFromRequest(w, r)
		if !ok {
			return
		}

		ctx := context.WithValue(r.Context(), peerNameKey, peer.Name)
		next(w, r.WithContext(ctx))
	}
}

// withAdminAuth authenticates an admin peer by its source IP. It wraps
// a whole subtree, so it is a handler-level middleware.
func (a *API) withAdminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, ok := a.peerFromRequest(w, r)
		if !ok {
			return
		}
		if !peer.Admin {
			wire.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), peerNameKey, peer.Name)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withInviteAuth authenticates a pending invite by its source IP on
// the invite network.
func (a *API) withInviteAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ip == nil {
			wire.WriteError(w, http.StatusBadRequest, "unable to determine client IP")
			return
		}

		invite, err := a.service.GetInviteByIP(ip)
		if err != nil {
			wire.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), inviteKey, invite)
		next(w, r.WithContext(ctx))
	}
}

// peerFromRequest resolves the calling peer from the connection's
// source IP, writing the failure response when it cannot.
func (a *API) peerFromRequest(
	w http.ResponseWriter,
	r *http.Request,
) (
	*server.Peer,
	bool,
) {
	ip := clientIP(r)
	if ip == nil {
		wire.WriteError(w, http.StatusBadRequest, "unable to determine client IP")
		return nil, false
	}

	peer, err := a.service.GetPeerByIP(ip)
	if err != nil {
		wire.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}

	return peer, true
}

// clientIP extracts the connection's source address. The API is only
// reachable over WireGuard, so the source IP is cryptographically tied
// to a peer; forwarding headers are deliberately ignored.
func clientIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// decodeJSON decodes a request body into dest.
func decodeJSON(r *http.Request, dest any) error {
	return json.NewDecoder(r.Body).Decode(dest)
}

// writeServiceError translates a service error into an HTTP response.
func writeServiceError(
	w http.ResponseWriter,
	err error,
) {
	wire.WriteError(w, httpStatusFromError(err), err.Error())
}

// httpStatusFromError is the API contract for service failures.
func httpStatusFromError(
	err error,
) int {
	switch {
	case errors.Is(err, server.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, server.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, server.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

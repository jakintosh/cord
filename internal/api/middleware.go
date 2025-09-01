package api

import (
	"context"
	"net"
	"net/http"
	"strings"
)

func (api *API) withMainAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get client IP
		ip := clientIP(r)
		if ip == nil {
			writeError(w, http.StatusBadRequest, "unable to determine client IP")
			return
		}

		// get peer name from ip
		peer, err := api.server.GetPeerByIP(ip)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// put peer name into context
		ctx := context.WithValue(r.Context(), "peerName", peer.Name)
		next(w, r.WithContext(ctx))
	}
}

func (api *API) withInviteAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get client IP
		ip := clientIP(r)
		if ip == nil {
			writeError(w, http.StatusBadRequest, "unable to determine client IP")
			return
		}

		// get invite from ip
		invite, err := api.server.GetInviteByIP(ip)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// put peer name into context
		ctx := context.WithValue(r.Context(), "inviteKey", invite.PublicKey)
		next(w, r.WithContext(ctx))
	}
}

func (api *API) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get client IP
		ip := clientIP(r)
		if ip == nil {
			writeError(w, http.StatusBadRequest, "unable to determine client IP")
			return
		}

		// get peer name from ip
		peer, err := api.server.GetPeerByIP(ip)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// make sure caller has admin privileges
		if !peer.Admin {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// put peer name into context
		ctx := context.WithValue(r.Context(), "peerName", peer)
		next(w, r.WithContext(ctx))
	}
}

func clientIP(r *http.Request) net.IP {
	host := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		host = strings.TrimSpace(parts[0])
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return net.ParseIP(host)
}

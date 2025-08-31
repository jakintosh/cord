package api

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// API will eventually hold api state and service interfaces
type API struct {
	server server.Context
}

// APIResponse is the standard envelope for all responses
type APIResponse struct {
	Error *APIError `json:"error"`
	Data  any       `json:"data"`
}

// APIError describes an error returned by the API
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func Init(server server.Context) *API {
	return &API{server}
}

func (api *API) withMainAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get client IP
		ip := clientIP(r)
		if ip == nil {
			writeError(w, http.StatusBadRequest, "unable to determine client IP")
			return
		}

		// get peer name from ip
		peer, err := api.server.PeerGetByIP(ip)
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
		peer, err := api.server.PeerGetByIP(ip)
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

// clientIP helper retained in case future logic needs it
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

// writeError sends an error envelope with a status code
func writeError(
	w http.ResponseWriter,
	code int,
	message string,
) {
	w.WriteHeader(code)
	writeJSON(w, APIResponse{
		Error: &APIError{
			Code:    code,
			Message: message,
		},
		Data: nil,
	})
}

// writeData sends a data envelope with a status code
func writeData(
	w http.ResponseWriter,
	code int,
	data any,
) {
	if data != nil {
		w.WriteHeader(code)
		writeJSON(w, APIResponse{
			Error: nil,
			Data:  data,
		})
	} else {
		w.WriteHeader(code)
	}
}

// writeJSON writes a raw JSON value, setting the content-type
func writeJSON(
	w http.ResponseWriter,
	v any,
) {
	if v == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		log.Printf("failed to write JSON: %v\n", err)
	}
}

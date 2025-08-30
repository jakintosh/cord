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

func NewAPI(server server.Context) *API {
	return &API{server}
}

// Auth middlewares are placeholders; they pass-through for now.
func (a *API) withMainAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ip == nil {
			writeError(w, http.StatusBadRequest, "unable to determine client IP")
			return
		}

		peerName, err := a.server.PeerGet(ip.String())
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), "peerName", peerName)
		next(w, r.WithContext(ctx))
	}
}
func (a *API) withInviteAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { next(w, r) }
}
func (a *API) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { next(w, r) }
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

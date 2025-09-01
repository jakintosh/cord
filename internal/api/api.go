package api

import (
	"encoding/json"
	"log"
	"net/http"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

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

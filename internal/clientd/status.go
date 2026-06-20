package clientd

import (
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

type StatusResponse struct {
	Status string `json:"status"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	wire.WriteData(w, http.StatusOK, StatusResponse{Status: "ok"})
}

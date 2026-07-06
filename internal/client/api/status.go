package api

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

type StatusDTO struct {
	Status   string       `json:"status"`
	Version  string       `json:"version"`
	Networks []NetworkDTO `json:"networks"`
}

func (a *API) handleStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	dtos, err := a.listNetworkDTOs()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, StatusDTO{
		Status:   "ok",
		Version:  a.version,
		Networks: dtos,
	})
}

func (c *Client) Status(
	ctx context.Context,
) (
	StatusDTO,
	error,
) {
	var result StatusDTO
	return result, c.wire.Get(ctx, "/status", &result)
}

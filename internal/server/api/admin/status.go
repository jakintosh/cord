package admin

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type NetworkStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
}

type Status struct {
	Status   string          `json:"status"`
	Version  string          `json:"version"`
	Networks []NetworkStatus `json:"networks"`
}

func networkStatusFromService(status service.NetworkStatus) NetworkStatus {
	return NetworkStatus{
		Name:    status.Name,
		Enabled: status.Enabled,
		Running: status.Running,
	}
}

func (a *API) handleGetStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	status, err := a.service.Status()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networks := make([]NetworkStatus, len(status.Networks))
	for i, network := range status.Networks {
		networks[i] = networkStatusFromService(network)
	}

	wire.WriteData(w, http.StatusOK, Status{
		Status:   "ok",
		Version:  a.version,
		Networks: networks,
	})
}

func (c *Client) Status(
	ctx context.Context,
) (
	Status,
	error,
) {
	var result Status
	return result, c.wire.Get(ctx, "/status", &result)
}

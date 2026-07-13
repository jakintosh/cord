package api

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

type NetworkStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
}

type InstallStatus struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type Status struct {
	Status   string          `json:"status"`
	Version  string          `json:"version"`
	Networks []NetworkStatus `json:"networks"`
	Installs []InstallStatus `json:"installs"`
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
	for i, n := range status.Networks {
		networks[i] = NetworkStatus{
			Name:    n.Name,
			Enabled: n.Enabled,
			Running: n.Running,
		}
	}

	installs := make([]InstallStatus, len(status.Installs))
	for i, inst := range status.Installs {
		installs[i] = InstallStatus{
			Name:  inst.Name,
			State: inst.State,
		}
	}

	wire.WriteData(w, http.StatusOK, Status{
		Status:   "ok",
		Version:  a.version,
		Networks: networks,
		Installs: installs,
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

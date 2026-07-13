package api

import (
	"context"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

type Status struct {
	Status   string          `json:"status"`
	Version  string          `json:"version"`
	Installs []InstallStatus `json:"installs"`
	Networks []NetworkStatus `json:"networks"`
}

type InstallStatus struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type NetworkStatus struct {
	Name    string        `json:"name"`
	Enabled bool          `json:"enabled"`
	Running bool          `json:"running"`
	Sync    RefreshStatus `json:"sync"`
	Scan    RefreshStatus `json:"scan"`
	Report  RefreshStatus `json:"report"`
}

type RefreshStatus struct {
	CadenceSeconds int64   `json:"cadence_seconds"`
	LastRunAt      *string `json:"last_run_at"`
}

func refreshStatusFromService(
	status service.RefreshStatus,
) RefreshStatus {
	result := RefreshStatus{
		CadenceSeconds: int64(status.Cadence / time.Second),
	}
	if !status.LastRunAt.IsZero() {
		formatted := status.LastRunAt.Format(time.RFC3339)
		result.LastRunAt = &formatted
	}
	return result
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
			Sync:    refreshStatusFromService(n.Sync),
			Scan:    refreshStatusFromService(n.Scan),
			Report:  refreshStatusFromService(n.Report),
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

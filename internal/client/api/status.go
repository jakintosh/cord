package api

import (
	"context"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

type Status struct {
	Status   string          `json:"status"`
	Version  string          `json:"version"`
	Installs []InstallStatus `json:"installs"`
	Networks []NetworkStatus `json:"networks"`
}

// InstallStatus reports an in-progress install. Completed installs are
// consumed into networks at confirm, so only mid-install records appear.
type InstallStatus struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func installStatusFromService(
	install service.Install,
) InstallStatus {
	return InstallStatus{
		Name:  install.Name,
		State: install.Phase,
	}
}

// NetworkStatus reports a network's persisted intent alongside what the
// daemon is actually doing. Reason explains any divergence — a network
// that is enabled but not running says why here.
type NetworkStatus struct {
	Name    string        `json:"name"`
	Enabled bool          `json:"enabled"`
	Running bool          `json:"running"`
	Reason  string        `json:"reason,omitempty"`
	Sync    RefreshStatus `json:"sync"`
	Scan    RefreshStatus `json:"scan"`
	Report  RefreshStatus `json:"report"`
}

func networkStatusFromRuntime(
	status runtime.NetworkStatus,
) NetworkStatus {
	return NetworkStatus{
		Name:    status.Name,
		Enabled: status.Enabled,
		Running: status.Running,
		Reason:  status.Reason,
		Sync:    refreshStatusFromRuntime(status.Sync),
		Scan:    refreshStatusFromRuntime(status.Scan),
		Report:  refreshStatusFromRuntime(status.Report),
	}
}

type RefreshStatus struct {
	CadenceSeconds int64   `json:"cadence_seconds"`
	LastRunAt      *string `json:"last_run_at"`
}

func refreshStatusFromRuntime(
	status runtime.RefreshStatus,
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
	status, err := a.runtime.Status()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	installs, err := a.service.ListInstalls()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkStatuses := make([]NetworkStatus, len(status.Networks))
	for i, network := range status.Networks {
		networkStatuses[i] = networkStatusFromRuntime(network)
	}

	installStatuses := make([]InstallStatus, len(installs))
	for i, install := range installs {
		installStatuses[i] = installStatusFromService(*install)
	}

	wire.WriteData(w, http.StatusOK, Status{
		Status:   "ok",
		Version:  a.version,
		Networks: networkStatuses,
		Installs: installStatuses,
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

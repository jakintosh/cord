package api

import (
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (a *API) handleGetStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	status, err := a.runtime.GetStatus()
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
		Health:   status.Health,
		Version:  a.version,
		Networks: networkStatuses,
		Installs: installStatuses,
	})
}

func installStatusFromService(
	install service.Install,
) InstallStatus {
	return InstallStatus{
		Name:  install.Name,
		State: install.Phase,
	}
}

func networkStatusFromRuntime(
	status runtime.NetworkStatus,
) NetworkStatus {
	return NetworkStatus{
		Name:      status.Name,
		Enabled:   status.Enabled,
		Running:   status.Running,
		Reason:    status.Reason,
		Health:    status.Health,
		Reconcile: activityStatusFromRuntime(status.Reconcile),
		Sync:      activityStatusFromRuntime(status.Sync),
		Scan:      activityStatusFromRuntime(status.Scan),
		Report:    activityStatusFromRuntime(status.Report),
	}
}

func activityStatusFromRuntime(
	status runtime.ActivityStatus,
) ActivityStatus {
	result := ActivityStatus{
		IntervalSeconds: int64(status.Interval / time.Second),
		Error:           status.Error,
	}
	if !status.LastAttemptAt.IsZero() {
		formatted := status.LastAttemptAt.Format(time.RFC3339)
		result.LastAttemptAt = &formatted
	}
	if !status.LastSuccessAt.IsZero() {
		formatted := status.LastSuccessAt.Format(time.RFC3339)
		result.LastSuccessAt = &formatted
	}
	return result
}

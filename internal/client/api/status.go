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
	status, err := a.runtime.GetStatus(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkStatuses := make([]NetworkStatus, len(status.Networks))
	for i, network := range status.Networks {
		networkStatuses[i] = networkStatusFromRuntime(network)
	}

	installStatuses := make([]InstallStatus, len(status.Installs))
	for i, install := range status.Installs {
		installStatuses[i] = installStatusFromService(*install)
	}

	snapshot := resourceSnapshotFromRuntime(status.Snapshot)

	wire.WriteData(w, http.StatusOK, Status{
		Snapshot: snapshot,
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
	if !status.NextAttemptAt.IsZero() {
		formatted := status.NextAttemptAt.Format(time.RFC3339)
		result.NextAttemptAt = &formatted
	}
	return result
}

func resourceSnapshotFromRuntime(
	status runtime.ResourceSnapshot,
) ResourceSnapshot {
	result := ResourceSnapshot{
		Kind:   status.Kind,
		Name:   status.Name,
		State:  string(status.State),
		Target: string(status.Target),
		Reason: status.Reason,
		Health: status.Health,
		Error:  status.Error,
	}
	if len(status.Activities) > 0 {
		result.Activities = make(map[string]ActivityStatus, len(status.Activities))
		for name, activity := range status.Activities {
			result.Activities[name] = activityStatusFromRuntime(activity)
		}
	}
	for _, child := range status.Children {
		result.Children = append(result.Children, resourceSnapshotFromRuntime(child))
	}
	return result
}

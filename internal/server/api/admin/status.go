package admin

import (
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
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

	networks := make([]NetworkStatus, len(status.Networks))
	for i, network := range status.Networks {
		networks[i] = networkStatusFromRuntime(network)
	}

	wire.WriteData(w, http.StatusOK, Status{
		Status:   "ok",
		Health:   status.Health,
		Version:  a.version,
		Networks: networks,
	})
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
		MainAPI:   activityStatusFromRuntime(status.MainAPI),
		InviteAPI: activityStatusFromRuntime(status.InviteAPI),
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

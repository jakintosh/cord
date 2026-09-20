package admin

import (
	"fmt"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
)

func (a *API) handleGetStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	if status, err := a.runtime.GetOverview(
		r.Context(),
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		networks := make([]NetworkStatus, len(status.Networks))
		for i, network := range status.Networks {
			networks[i] = networkStatusFromRuntime(network)
		}

		statusDTO := Status{
			Status:   "ok",
			Health:   status.Health,
			Version:  a.version,
			Networks: networks,
		}
		wire.WriteData(w, http.StatusOK, statusDTO)
	}
}

func networkStatusFromRuntime(
	status runtime.NetworkOverview,
) NetworkStatus {
	result := NetworkStatus{
		Name:    status.Name,
		Enabled: status.Enabled,
		Running: status.Running,
		Reason:  status.Reason,
		Health:  status.Health,
	}
	if snapshot := status.Status; snapshot != nil {
		result.Reconcile = activityStatusFromRuntime(snapshot.Reconcile)
		result.MainAPI = activityStatusFromResource(snapshot.Main.HTTP)
		result.InviteAPI = activityStatusFromResource(snapshot.Invite.HTTP)
		result.DNS = activityStatusFromResource(snapshot.Main.DNS)
	}
	return result
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

// activityStatusFromResource preserves the admin API's existing activity fields
// while the runtime reports current resource state.
func activityStatusFromResource(
	status resource.Status,
) ActivityStatus {
	if status.State == resource.Disabled || status.State == resource.Running {
		return ActivityStatus{}
	}
	if status.Error != nil {
		return ActivityStatus{Error: status.Error.Error()}
	}
	return ActivityStatus{Error: fmt.Sprintf("resource %s", status.State)}
}

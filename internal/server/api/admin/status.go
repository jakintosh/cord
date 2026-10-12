package admin

import (
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
	if status, err := a.runtime.GetStatus(
		r.Context(),
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		networks := make([]NetworkStatus, len(status.Networks))
		for i, network := range status.Networks {
			networks[i] = networkStatusFromRuntime(network)
		}

		statusDTO := Status{
			Snapshot: snapshotFromRuntime(status.Snapshot),
			Status:   "ok",
			Health:   string(status.Health),
			Version:  a.version,
			Networks: networks,
		}
		wire.WriteData(w, http.StatusOK, statusDTO)
	}
}

func networkStatusFromRuntime(
	status runtime.NetworkStatus,
) NetworkStatus {
	return NetworkStatus{
		Name:      status.Name,
		Enabled:   status.Enabled,
		Reason:    status.Reason,
		Health:    string(status.Health),
		Reconcile: activityStatusFromRuntime(status.Reconcile),
	}
}

func activityStatusFromRuntime(
	status resource.ActivityStatus,
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

func snapshotFromRuntime(
	status resource.Snapshot,
) ResourceSnapshot {
	result := ResourceSnapshot{
		Kind:   status.Kind,
		Name:   status.Name,
		State:  string(status.State),
		Target: string(status.Target),
		Health: string(status.Health),
		Error:  status.Error,
	}

	if len(status.Activities) > 0 {
		result.Activities = make(map[string]ActivityStatus, len(status.Activities))
		for name, activity := range status.Activities {
			result.Activities[name] = activityStatusFromRuntime(activity)
		}
	}

	for _, child := range status.Children {
		result.Children = append(result.Children, snapshotFromRuntime(child))
	}

	return result
}

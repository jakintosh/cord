package admin

import (
	"context"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
)

type Status struct {
	Status   string          `json:"status"`
	Health   string          `json:"health"`
	Version  string          `json:"version"`
	Networks []NetworkStatus `json:"networks"`
}

// NetworkStatus reports persisted intent, actual process state, and the
// health of a network's runtime work. Reason explains state divergence.
type NetworkStatus struct {
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Running   bool           `json:"running"`
	Reason    string         `json:"reason,omitempty"`
	Health    string         `json:"health"`
	Reconcile ActivityStatus `json:"reconcile"`
	MainAPI   ActivityStatus `json:"main_api"`
	InviteAPI ActivityStatus `json:"invite_api"`
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

type ActivityStatus struct {
	IntervalSeconds int64   `json:"interval_seconds,omitempty"`
	LastAttemptAt   *string `json:"last_attempt_at"`
	LastSuccessAt   *string `json:"last_success_at"`
	Error           string  `json:"error,omitempty"`
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

func (a *API) handleGetStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	status, err := a.runtime.Status()
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

func (c *Client) Status(
	ctx context.Context,
) (
	Status,
	error,
) {
	var result Status
	return result, c.wire.Get(ctx, "/status", &result)
}

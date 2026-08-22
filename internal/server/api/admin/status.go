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
	Version  string          `json:"version"`
	Networks []NetworkStatus `json:"networks"`
}

// NetworkStatus reports a network's persisted intent alongside what the
// daemon is actually doing. Reason explains any divergence — a network
// that is enabled but not running says why here.
type NetworkStatus struct {
	Name      string          `json:"name"`
	Enabled   bool            `json:"enabled"`
	Running   bool            `json:"running"`
	Reason    string          `json:"reason,omitempty"`
	Reconcile ReconcileStatus `json:"reconcile"`
}

func networkStatusFromRuntime(
	status runtime.NetworkStatus,
) NetworkStatus {
	return NetworkStatus{
		Name:      status.Name,
		Enabled:   status.Enabled,
		Running:   status.Running,
		Reason:    status.Reason,
		Reconcile: reconcileStatusFromRuntime(status.Reconcile),
	}
}

type ReconcileStatus struct {
	MaxIntervalSeconds int64   `json:"max_interval_seconds"`
	LastRunAt          *string `json:"last_run_at"`
}

func reconcileStatusFromRuntime(
	status runtime.ReconcileStatus,
) ReconcileStatus {
	result := ReconcileStatus{
		MaxIntervalSeconds: int64(status.MaxInterval / time.Second),
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

	networks := make([]NetworkStatus, len(status.Networks))
	for i, network := range status.Networks {
		networks[i] = networkStatusFromRuntime(network)
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

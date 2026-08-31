package client

import "context"

// Status describes the client daemon and its managed networks.
type Status struct {
	Status   string          `json:"status"`
	Health   string          `json:"health"`
	Version  string          `json:"version"`
	Installs []InstallStatus `json:"installs"`
	Networks []NetworkStatus `json:"networks"`
}

// InstallStatus reports an in-progress installation.
type InstallStatus struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// NetworkStatus describes persisted intent and live network state.
type NetworkStatus struct {
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Running   bool           `json:"running"`
	Reason    string         `json:"reason,omitempty"`
	Health    string         `json:"health"`
	Reconcile ActivityStatus `json:"reconcile"`
	Sync      ActivityStatus `json:"sync"`
	Scan      ActivityStatus `json:"scan"`
	Report    ActivityStatus `json:"report"`
}

// ActivityStatus describes one periodic daemon activity.
type ActivityStatus struct {
	IntervalSeconds int64   `json:"interval_seconds,omitempty"`
	LastAttemptAt   *string `json:"last_attempt_at"`
	LastSuccessAt   *string `json:"last_success_at"`
	Error           string  `json:"error,omitempty"`
}

// Status returns the daemon's current status.
func (c *Client) Status(
	ctx context.Context,
) (
	Status,
	error,
) {
	var result Status
	return result, c.wire.Get(ctx, "/status", &result)
}

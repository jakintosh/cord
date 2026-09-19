package client

import "context"

// Status describes the client daemon and its managed networks. Resource includes
// owned children awaiting cleanup, even after their saved records are deleted.
// If saved-state reads fail, Resource.Error describes the failure and Health is
// degraded; the available resource tree is still returned.
type Status struct {
	Snapshot ResourceSnapshot `json:"resource"`
	Status   string           `json:"status"`
	Health   string           `json:"health"`
	Version  string           `json:"version"`
	Installs []InstallStatus  `json:"installs"`
	Networks []NetworkStatus  `json:"networks"`
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
	Reason    string         `json:"reason,omitempty"`
	Health    string         `json:"health"`
	Reconcile ActivityStatus `json:"reconcile"`
	Sync      ActivityStatus `json:"sync"`
	Scan      ActivityStatus `json:"scan"`
	Report    ActivityStatus `json:"report"`
}

// ActivityStatus describes one periodic daemon activity.
type ActivityStatus struct {
	NextAttemptAt   *string `json:"next_attempt_at,omitempty"`
	IntervalSeconds int64   `json:"interval_seconds,omitempty"`
	LastAttemptAt   *string `json:"last_attempt_at"`
	LastSuccessAt   *string `json:"last_success_at"`
	Error           string  `json:"error,omitempty"`
}

// ResourceSnapshot describes an owned resource and its immediate children.
// State is observed availability; Target is the owner's intended state.
// Health includes activity failures and unhealthy descendants. Status collection
// is serialized with runtime operations and never repairs resources.
type ResourceSnapshot struct {
	Kind       string                    `json:"kind"`
	Name       string                    `json:"name,omitempty"`
	State      string                    `json:"state"`
	Target     string                    `json:"target"`
	Health     string                    `json:"health"`
	Error      string                    `json:"error,omitempty"`
	Activities map[string]ActivityStatus `json:"activities,omitempty"`
	Children   []ResourceSnapshot        `json:"children,omitempty"`
}

// Status returns the daemon's current status. It waits behind active runtime work.
func (c *Client) Status(
	ctx context.Context,
) (
	Status,
	error,
) {
	var result Status
	return result, c.wire.Get(ctx, "/status", &result)
}

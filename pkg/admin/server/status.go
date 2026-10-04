package server

import "context"

// Status describes the server daemon and its managed networks.
type Status struct {
	Snapshot ResourceSnapshot `json:"resource"`
	Status   string           `json:"status"`
	Health   string           `json:"health"`
	Version  string           `json:"version"`
	Networks []NetworkStatus  `json:"networks"`
}

// NetworkStatus describes persisted intent and live network state.
type NetworkStatus struct {
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Reason    string         `json:"reason,omitempty"`
	Health    string         `json:"health"`
	Reconcile ActivityStatus `json:"reconcile"`
}

// ActivityStatus describes one periodic daemon activity.
type ActivityStatus struct {
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

// Status returns current status, waiting behind runtime work. Storage errors
// degrade the snapshot without hiding owned resources awaiting cleanup.
func (c *Client) Status(
	ctx context.Context,
) (
	Status,
	error,
) {
	var result Status
	return result, c.wire.Get(ctx, "/status", &result)
}

package runtime

import (
	"context"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// RuntimeStatus observes owned resources and full-pass activity without storage reads.
type RuntimeStatus struct {
	Resource  resource.Status
	Reconcile ActivityStatus
	Networks  map[string]NetworkStatus
}

// RuntimeOverview joins persisted network intent with runtime and child health.
type RuntimeOverview struct {
	Health    string
	Resource  resource.Status
	Reconcile ActivityStatus
	Networks  []NetworkOverview
}

// NetworkOverview is the operator-facing state of a server network:
// persisted intent, actual process state, and the health of running work.
type NetworkOverview struct {
	Name    string
	Enabled bool
	Running bool // The runtime owns an instance; its resources may be unhealthy.
	Reason  string
	Health  string
	Status  *NetworkStatus // Nil when there is no running instance.
}

// ActivityStatus describes the latest result of one runtime activity.
// Interval is zero for work without a periodic schedule.
type ActivityStatus struct {
	Interval      time.Duration
	LastAttemptAt time.Time
	LastSuccessAt time.Time
	Error         string
}

func (s *ActivityStatus) record(
	at time.Time,
	err error,
) {
	s.LastAttemptAt = at
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.LastSuccessAt = at
	s.Error = ""
}

// Status collects runtime availability, full-pass activity, and child snapshots.
func (r *Runtime) Status(
	ctx context.Context,
) (
	RuntimeStatus,
	error,
) {
	return executeValue(r, ctx, func() (RuntimeStatus, error) {
		networks := make(map[string]NetworkStatus, len(r.running))
		for name, network := range r.running {
			networks[name] = network.status()
		}

		return RuntimeStatus{
			Resource:  resource.Status{State: resource.Running},
			Reconcile: r.reconcileStatus,
			Networks:  networks,
		}, nil
	})
}

// GetOverview joins the persisted intent with what this process is actually
// doing. It is the single source of operator-facing state.
func (r *Runtime) GetOverview(
	ctx context.Context,
) (
	RuntimeOverview,
	error,
) {
	return executeValue(r, ctx, func() (RuntimeOverview, error) {
		networks, err := r.service.ListNetworks()
		if err != nil {
			return RuntimeOverview{}, err
		}

		statuses := make([]NetworkOverview, len(networks))
		for i, network := range networks {
			statuses[i] = r.networkOverview(network)
		}

		state := resource.Status{State: resource.Running}
		health := statusHealth(statuses)
		if r.reconcileStatus.Error != "" {
			health = HealthDegraded
		}

		return RuntimeOverview{
			Health:    health,
			Resource:  state,
			Reconcile: r.reconcileStatus,
			Networks:  statuses,
		}, nil
	})
}

// networkOverview joins one persisted record with the running set.
func (r *Runtime) networkOverview(
	network *service.Network,
) NetworkOverview {
	running, isRunning := r.running[network.Name]
	status := NetworkOverview{
		Name:    network.Name,
		Enabled: network.Enabled,
		Running: isRunning,
		Reason:  r.reasons[network.Name],
	}
	if isRunning {
		snapshot := running.status()
		status.Status = &snapshot
	}

	status.Health = networkHealth(status)
	return status
}

func networkHealth(
	status NetworkOverview,
) string {
	if status.Reason != "" {
		return HealthDegraded
	}

	if !status.Enabled && !status.Running {
		return HealthInactive
	}

	if status.Enabled != status.Running {
		return HealthDegraded
	}

	if status.Status == nil || status.Status.Reconcile.Error != "" {
		return HealthDegraded
	}

	for _, plane := range []PlaneStatus{
		status.Status.Main,
		status.Status.Invite,
	} {
		for _, child := range []resource.Status{
			plane.WireGuard,
			plane.DNS,
			plane.HTTP,
		} {
			if child.Error != nil || (child.State != resource.Running && child.State != resource.Disabled) {
				return HealthDegraded
			}
		}
	}

	return HealthHealthy
}

func statusHealth(
	statuses []NetworkOverview,
) string {
	for _, status := range statuses {
		if status.Health == HealthDegraded {
			return HealthDegraded
		}
	}

	return HealthHealthy
}

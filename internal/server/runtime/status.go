package runtime

import (
	"context"
	"fmt"
	"slices"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// Status joins saved network intent with a detached tree of owned resources.
type Status struct {
	Snapshot resource.Snapshot
	Health   resource.Health
	Networks []NetworkStatus
}

// NetworkStatus describes saved intent and the health of its owned resource.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Reason    string
	Health    resource.Health
	Reconcile resource.ActivityStatus
}

// GetStatus waits behind runtime work and observes resources without repairing them.
// Storage failures degrade the response without hiding the ownership tree.
func (r *Runtime) GetStatus(
	ctx context.Context,
) (
	Status,
	error,
) {
	return executeValue(r, ctx, func() (Status, error) {
		snapshot := r.status()
		networks, err := r.service.ListNetworks()
		if err != nil {
			snapshot.Error = fmt.Sprintf("read saved state: %v", err)
			snapshot.UpdateHealth()
		}

		owned := make(map[string]*resource.Snapshot, len(snapshot.Children))
		for i := range snapshot.Children {
			child := &snapshot.Children[i]
			owned[child.Name] = child
		}
		statuses := make([]NetworkStatus, len(networks))
		for i, network := range networks {
			statuses[i] = r.networkStatus(network, owned[network.Name])
			if statuses[i].Health == resource.HealthDegraded {
				snapshot.Health = resource.HealthDegraded
			}
		}

		return Status{
			Snapshot: snapshot,
			Health:   snapshot.Health,
			Networks: statuses,
		}, nil
	})
}

func (r *Runtime) status() resource.Snapshot {
	status := resource.Snapshot{
		Kind:   "runtime",
		State:  resource.Running,
		Target: resource.Running,
		Activities: map[string]resource.ActivityStatus{
			"reconcile": r.reconcileStatus,
		},
	}

	var names []string
	for name := range r.running {
		names = append(names, name)
	}

	slices.Sort(names)
	for _, name := range names {
		childStatus := r.running[name].status()
		status.Children = append(status.Children, childStatus)
	}

	status.UpdateHealth()

	return status
}

func (r *Runtime) networkStatus(
	network *service.Network,
	snapshot *resource.Snapshot,
) NetworkStatus {
	status := NetworkStatus{
		Name:    network.Name,
		Enabled: network.Enabled,
		Reason:  r.reasons[network.Name],
		Health:  resource.HealthInactive,
	}

	if snapshot != nil {
		status.Health = snapshot.Health
		status.Reconcile = snapshot.Activities["reconcile"]
	}

	if status.Reason != "" || network.Enabled != (snapshot != nil) {
		status.Health = resource.HealthDegraded
	}

	return status
}

// resourceSnapshot adapts a leaf observation without changing its resource.
func resourceSnapshot(
	kind string,
	status resource.Status,
	target resource.State,
) resource.Snapshot {
	snapshot := resource.Snapshot{
		Kind:   kind,
		State:  status.State,
		Target: target,
	}

	if status.Error != nil {
		snapshot.Error = status.Error.Error()
	}

	snapshot.UpdateHealth()

	return snapshot
}

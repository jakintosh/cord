package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
)

// Status collects network state and pending installations in one runtime command.
type Status struct {
	Snapshot resource.Snapshot
	Health   resource.Health
	Networks []NetworkStatus
	Installs []*service.Install
}

// NetworkStatus is the operator-facing state of an installed network:
// persisted intent, actual process state, and the health of running work.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Reason    string
	Health    resource.Health
	Reconcile resource.ActivityStatus
	Sync      resource.ActivityStatus
	Scan      resource.ActivityStatus
	Report    resource.ActivityStatus
}

// GetStatus joins the persisted intent with what this process is actually
// doing. Storage read failures are reported on the resource root so that
// owned resources remain observable when their saved summaries are unavailable.
func (r *Runtime) GetStatus(
	ctx context.Context,
) (
	Status,
	error,
) {
	return executeValue(r, ctx, func() (Status, error) {
		tree := r.status()
		records, networkErr := r.service.ListNetworks()
		installs, installErr := r.service.ListInstalls()

		if err := errors.Join(networkErr, installErr); err != nil {
			tree.Error = fmt.Sprintf("read saved state: %v", err)
			tree.UpdateHealth()
		}

		owned := make(map[string]*resource.Snapshot, len(tree.Children))
		for i := range tree.Children {
			child := &tree.Children[i]
			if child.Kind == "network" {
				owned[child.Name] = child
			}
		}
		statuses := make([]NetworkStatus, len(records))
		for i, record := range records {
			statuses[i] = r.networkStatus(record, owned[record.Name])
			if statuses[i].Health == resource.HealthDegraded {
				tree.Health = resource.HealthDegraded
			}
		}

		return Status{
			Snapshot: tree,
			Health:   tree.Health,
			Networks: statuses,
			Installs: installs,
		}, nil
	})
}

func (r *Runtime) status() resource.Snapshot {
	status := resource.Snapshot{
		Kind:   "runtime",
		State:  resource.Running,
		Target: resource.Running,
		Activities: map[string]resource.ActivityStatus{
			"reconcile":   r.reconcileStatus,
			"cleanup":     r.cleanupStatus,
			"dns-cleanup": r.dnsCleanupStatus,
		},
	}

	var names []string
	for name := range r.running {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		status.Children = append(status.Children, r.running[name].status())
	}

	names = nil
	for name := range r.enrollments {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		status.Children = append(status.Children, r.enrollments[name].status())
	}

	status.UpdateHealth()
	return status
}

// getNetworkStatus reports the status of one network by name.
func (r *Runtime) getNetworkStatus(
	name string,
) (
	NetworkStatus,
	error,
) {
	record, err := r.service.GetNetwork(name)
	if err != nil {
		return NetworkStatus{}, err
	}

	var snapshot *resource.Snapshot
	if network := r.running[name]; network != nil {
		status := network.status()
		snapshot = &status
	}
	return r.networkStatus(record, snapshot), nil
}

// networkStatus joins saved intent with an already collected network snapshot.
func (r *Runtime) networkStatus(
	record *service.Network,
	snapshot *resource.Snapshot,
) NetworkStatus {
	status := NetworkStatus{
		Name:    record.Name,
		Enabled: record.Enabled,
		Reason:  r.reasons[record.Name],
		Health:  resource.HealthInactive,
		Sync:    resource.ActivityStatus{Interval: r.syncInterval},
		Scan:    resource.ActivityStatus{Interval: r.scanInterval},
		Report:  resource.ActivityStatus{Interval: r.reportInterval},
	}

	if snapshot != nil {
		status.Health = snapshot.Health
		status.Reconcile = snapshot.Activities["reconcile"]
		status.Sync = snapshot.Activities["sync"]
		status.Scan = snapshot.Activities["scan"]
		status.Report = snapshot.Activities["report"]
	}

	if status.Reason != "" || record.Enabled != (snapshot != nil) {
		status.Health = resource.HealthDegraded
	}

	return status
}

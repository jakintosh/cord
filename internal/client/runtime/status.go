package runtime

import (
	"slices"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
)

// ResourceSnapshot is a detached ownership tree collected by the runtime loop.
// State describes availability; Target describes the owner's intended lifetime.
// Health includes local activity failures and unhealthy descendants.
type ResourceSnapshot struct {
	Kind       string
	Name       string
	State      resource.State
	Target     resource.State
	Error      string
	Health     string
	Activities map[string]ActivityStatus
	Children   []ResourceSnapshot
}

func (r *Runtime) status() ResourceSnapshot {
	status := ResourceSnapshot{
		Kind:   "runtime",
		State:  resource.Running,
		Target: resource.Running,
		Activities: map[string]ActivityStatus{
			"reconcile": r.reconcileStatus,
			"cleanup":   r.cleanupStatus,
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

	status.updateHealth()
	return status
}

func (n *Network) status() ResourceSnapshot {
	target := resource.Running
	if n.closing {
		target = resource.Stopped
	}

	state := resource.Running
	if n.isStopped() {
		state = resource.Stopped
	}
	if n.cleanupStatus.Error != "" {
		state = resource.Failed
	}

	status := ResourceSnapshot{
		Kind:   "network",
		Name:   n.record.Name,
		State:  state,
		Target: target,
		Activities: map[string]ActivityStatus{
			"reconcile": n.reconcileStatus,
			"sync":      n.syncStatus,
			"scan":      n.scanStatus,
			"report":    n.reportStatus,
			"close":     n.cleanupStatus,
		},
		Children: []ResourceSnapshot{n.tunnel.status(target)},
	}
	status.updateHealth()
	return status
}

func (e *Enrollment) status() ResourceSnapshot {
	// Status commands run between operations. A retained enrollment is
	// awaiting cleanup, even when its saved installation has been deleted.
	status := ResourceSnapshot{
		Kind:   "enrollment",
		Name:   e.install.Name,
		State:  resource.Stopped,
		Target: resource.Stopped,
		Activities: map[string]ActivityStatus{
			"redeem":  e.redeemStatus,
			"confirm": e.confirmStatus,
			"close":   e.cleanupStatus,
		},
	}
	if e.cleanupStatus.Error != "" {
		status.State = resource.Failed
	}
	if e.tunnel != nil {
		status.Children = []ResourceSnapshot{
			e.tunnel.status(resource.Stopped),
		}
	}
	status.updateHealth()

	return status
}

func (s *ResourceSnapshot) updateHealth() {
	s.Health = HealthHealthy
	if s.State == resource.Stopped && s.Target == resource.Stopped {
		s.Health = HealthInactive
	}

	if s.Error != "" || s.State == resource.Failed || s.State != s.Target {
		s.Health = HealthDegraded
	}

	for name, activity := range s.Activities {
		// Completed operational work does not prevent intentional retirement.
		if s.Target == resource.Stopped && name != "close" {
			continue
		}
		if activity.Error != "" {
			s.Health = HealthDegraded
		}
	}

	for _, child := range s.Children {
		if child.Health == HealthDegraded {
			s.Health = HealthDegraded
		}
	}
}

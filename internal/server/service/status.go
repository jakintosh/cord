package service

import (
	"maps"
	"time"
)

// Status is a snapshot of every network known to the server daemon.
type Status struct {
	Networks []NetworkStatus
}

// NetworkStatus is the operator-facing state of a server network: its
// persisted enabled intent and whether it is actually running in this process.
type NetworkStatus struct {
	Name      string
	Enabled   bool
	Running   bool
	Reconcile ReconcileStatus
}

// ReconcileStatus describes the latest server reconciliation and the maximum
// interval used when no registration expiry schedules an earlier run.
type ReconcileStatus struct {
	MaxInterval time.Duration
	LastRunAt   time.Time
}

// Status returns all server networks using one bulk store read, then folds in
// live runtime state from a single snapshot of the in-memory running set.
func (s *Service) Status() (
	Status,
	error,
) {
	networks, err := s.store.ListNetworks()
	if err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	running := make(map[string]*Network, len(s.running))
	maps.Copy(running, s.running)
	s.mu.Unlock()

	statuses := make([]NetworkStatus, len(networks))
	for i, network := range networks {
		runtime, isRunning := running[network.Name]
		reconcile := ReconcileStatus{MaxInterval: defaultReconcileCap}
		if isRunning {
			reconcile.LastRunAt = runtime.lastReconcile()
		}
		statuses[i] = NetworkStatus{
			Name:      network.Name,
			Enabled:   network.Enabled,
			Running:   isRunning,
			Reconcile: reconcile,
		}
	}

	return Status{Networks: statuses}, nil
}

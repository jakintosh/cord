package service

import (
	"maps"
	"time"
)

// Status is a snapshot of every installed network and in-progress
// install known to the daemon.
type Status struct {
	Networks []NetworkStatus
	Installs []InstallStatus
}

// InstallStatus is the operator-facing status of an in-progress
// install. Completed installs are consumed into networks at confirm, so
// only mid-install records appear here.
type InstallStatus struct {
	Name  string
	State string
}

// NetworkStatus is the operator-facing status of an installed network:
// its persisted enabled intent and its live runtime state.
type NetworkStatus struct {
	Name    string
	Enabled bool
	Running bool
	Sync    RefreshStatus
	Scan    RefreshStatus
	Report  RefreshStatus
}

// RefreshStatus describes one periodic runtime activity.
type RefreshStatus struct {
	Cadence   time.Duration
	LastRunAt time.Time
}

// Status returns a snapshot of all installed networks and in-progress
// installs. It reads durable state with exactly two bulk store calls
// and folds in live runtime state from the in-memory running set,
// avoiding any per-network store round trips.
func (s *Service) Status() (
	Status,
	error,
) {
	networks, err := s.store.ListNetworks()
	if err != nil {
		return Status{}, err
	}

	installs, err := s.store.ListInstalls()
	if err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	running := make(map[string]*Network, len(s.running))
	maps.Copy(running, s.running)
	s.mu.Unlock()

	networkStatuses := make([]NetworkStatus, len(networks))
	for i, nc := range networks {
		network, isRunning := running[nc.Name]
		syncStatus := RefreshStatus{Cadence: s.syncInterval}
		scanStatus := RefreshStatus{Cadence: s.scanInterval}
		reportStatus := RefreshStatus{Cadence: s.reportInterval}
		if isRunning {
			syncStatus.LastRunAt, scanStatus.LastRunAt, reportStatus.LastRunAt = network.lastRuns()
		}
		networkStatuses[i] = NetworkStatus{
			Name:    nc.Name,
			Enabled: nc.Enabled,
			Running: isRunning,
			Sync:    syncStatus,
			Scan:    scanStatus,
			Report:  reportStatus,
		}
	}

	installStatuses := make([]InstallStatus, len(installs))
	for i, inst := range installs {
		installStatuses[i] = InstallStatus{
			Name:  inst.Name,
			State: inst.Phase,
		}
	}

	return Status{
		Networks: networkStatuses,
		Installs: installStatuses,
	}, nil
}

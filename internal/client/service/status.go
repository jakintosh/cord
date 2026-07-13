package service

// NetworkStatus is the operator-facing status of an installed network:
// its persisted enabled intent and its live runtime state.
type NetworkStatus struct {
	Name    string
	Enabled bool
	Running bool
}

// InstallStatus is the operator-facing status of an in-progress
// install. Completed installs are consumed into networks at confirm, so
// only mid-install records appear here.
type InstallStatus struct {
	Name  string
	State string
}

// Status is a snapshot of every installed network and in-progress
// install known to the daemon.
type Status struct {
	Networks []NetworkStatus
	Installs []InstallStatus
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
	running := make(map[string]struct{}, len(s.running))
	for name := range s.running {
		running[name] = struct{}{}
	}
	s.mu.Unlock()

	networkStatuses := make([]NetworkStatus, len(networks))
	for i, nc := range networks {
		_, isRunning := running[nc.Name]
		networkStatuses[i] = NetworkStatus{
			Name:    nc.Name,
			Enabled: nc.Enabled,
			Running: isRunning,
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

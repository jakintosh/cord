package service

// NetworkStatus is the operator-facing state of a server network: its
// persisted enabled intent and whether it is actually running in this process.
type NetworkStatus struct {
	Name    string
	Enabled bool
	Running bool
}

// Status is a snapshot of every network known to the server daemon.
type Status struct {
	Networks []NetworkStatus
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
	running := make(map[string]struct{}, len(s.running))
	for name := range s.running {
		running[name] = struct{}{}
	}
	s.mu.Unlock()

	statuses := make([]NetworkStatus, len(networks))
	for i, network := range networks {
		_, isRunning := running[network.Name]
		statuses[i] = NetworkStatus{
			Name:    network.Name,
			Enabled: network.Enabled,
			Running: isRunning,
		}
	}

	return Status{Networks: statuses}, nil
}

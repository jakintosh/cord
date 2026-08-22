package service

import (
	"time"
)

// EndpointTTL bounds the local endpoint catalog. A candidate remains while
// it has been observed by either this client or the server within this
// window.
const EndpointTTL = 24 * time.Hour

// NetworkOptions controls local settings for a network installation.
type NetworkOptions struct {
	// ListenPort is the local WireGuard UDP port. Nil uses the service default.
	ListenPort *uint16
}

// Network is the permanent membership record: this client's identity on
// one overlay network, how to reach its server, and whether the network
// should be running. Complete at insert, with local settings and Enabled
// mutable after installation. It is inert domain data — the runtime owns
// all behavior.
type Network struct {
	Name          string
	PrivateKey    string
	InterfaceName string
	AssignedRoute string
	ListenPort    uint16
	Server        ServerInfo
	Enabled       bool
	CreatedAt     time.Time
}

// GetNetwork returns the persisted network record by name.
func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	return s.store.GetNetwork(name)
}

// ListNetworks returns every installed network, ordered by name.
func (s *Service) ListNetworks() (
	[]*Network,
	error,
) {
	return s.store.ListNetworks()
}

// SetNetworkEnabled records whether the network should be running. It is
// the whole of the enable/disable operation: the flag is persisted
// unconditionally and the runtime converges toward it, so a network that
// cannot start stays enabled and is retried rather than silently
// reverting to disabled.
func (s *Service) SetNetworkEnabled(
	name string,
	enabled bool,
) error {
	if err := s.store.SetNetworkEnabled(name, enabled); err != nil {
		return err
	}

	s.requestReconcile(name)

	s.log.Info(
		"network intent recorded",
		"network",
		name,
		"enabled",
		enabled,
	)

	return nil
}

// UpdateNetwork persists local network configuration. The stored record
// is the whole of the operation: a running network whose configuration
// no longer matches is restarted by the runtime's next converge.
func (s *Service) UpdateNetwork(
	name string,
	opts NetworkOptions,
) error {
	if opts.ListenPort == nil {
		return ErrInvalidInput
	}

	if err := s.store.UpdateNetwork(name, opts); err != nil {
		return err
	}

	s.requestReconcile(name)

	return nil
}

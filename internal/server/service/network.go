package service

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// defaultInviteCidr is the invite subnet used when CreateNetwork is
// called without an explicit Invite.Cidr.
const defaultInviteCidr = "172.16.10.0/24"

// inviteSuffix is the suffix appended to the network name for the
// invite device.
const inviteSuffix = "-i"

// Plane is the persisted configuration for one WireGuard plane
// (main or invite) within a network. It describes the interface name,
// address space, and port assignments.
type Plane struct {
	Name          string // WireGuard interface name
	Cidr          string // e.g. "10.42.0.0/16"
	WireguardPort uint16
	ApiPort       uint16
}

// Normalize validates a plane config and canonicalizes its network CIDR.
func (pc *Plane) Normalize() error {
	if err := wireguard.ValidateDeviceName(pc.Name); err != nil {
		return fmt.Errorf("%w: invalid device name: %v", ErrInvalidInput, err)
	}

	cidr, err := netaddr.ParseNetworkCIDR(pc.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid CIDR %q: %v", ErrInvalidInput, pc.Cidr, err)
	}

	pc.Cidr = cidr.String()

	return nil
}

// Network is the persisted identity of a server network. It holds the
// server's keypair, address space configuration, and network endpoints.
// It is inert domain data — the runtime owns all behavior.
type Network struct {
	Name       string
	PrivateKey string
	PublicKey  string
	ExternalIP string

	Main   Plane
	Invite Plane

	Enabled   bool
	CreatedAt time.Time
}

// Normalize applies defaults and validates the config. Zero-valued
// fields are filled with sensible defaults. Returns ErrInvalidInput
// or ErrCIDROverlap on failure.
func (n *Network) Normalize() error {
	if n.Name == "" {
		return fmt.Errorf("%w: network name required", ErrInvalidInput)
	}
	if n.ExternalIP == "" {
		return fmt.Errorf("%w: external IP required", ErrInvalidInput)
	}

	// Main plane defaults
	if n.Main.Name == "" {
		n.Main.Name = n.Name
	}
	if n.Main.WireguardPort == 0 {
		n.Main.WireguardPort = 51820
	}
	if n.Main.ApiPort == 0 {
		n.Main.ApiPort = 8080
	}

	// Invite plane defaults
	if n.Invite.Name == "" {
		n.Invite.Name = n.Name + inviteSuffix
	}
	if n.Invite.Cidr == "" {
		n.Invite.Cidr = defaultInviteCidr
	}
	if n.Invite.WireguardPort == 0 {
		n.Invite.WireguardPort = n.Main.WireguardPort + 1
	}
	if n.Invite.ApiPort == 0 {
		n.Invite.ApiPort = 8080
	}

	// Validate main plane
	if err := n.Main.Normalize(); err != nil {
		return fmt.Errorf("main: %w", err)
	}

	// Validate invite plane
	if err := n.Invite.Normalize(); err != nil {
		return fmt.Errorf("invite: %w", err)
	}

	// Cross-plane overlap check
	_, mainNet, err := net.ParseCIDR(n.Main.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid main CIDR %q: %v", ErrInvalidInput, n.Main.Cidr, err)
	}
	_, inviteNet, err := net.ParseCIDR(n.Invite.Cidr)
	if err != nil {
		return fmt.Errorf("%w: invalid invite CIDR %q: %v", ErrInvalidInput, n.Invite.Cidr, err)
	}
	if netaddr.Overlaps(mainNet, inviteNet) {
		return fmt.Errorf(
			"%w: invite CIDR %q overlaps main CIDR %q",
			ErrCIDROverlap, n.Invite.Cidr, n.Main.Cidr,
		)
	}

	return nil
}

// GetNetwork returns the persisted network record by name.
func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	nc, err := s.store.GetNetwork(name)
	if err != nil {
		return nil, fmt.Errorf("get network %q: %w", name, mapStoreError(err))
	}
	return nc, nil
}

// ListNetworks returns every persisted server network, ordered by name.
func (s *Service) ListNetworks() (
	[]*Network,
	error,
) {
	networks, err := s.store.ListNetworks()
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return networks, nil
}

// CreateNetwork defines a new server network: validates the config,
// generates the server keypair, and persists the record. Zero-valued
// fields in the Planes mean "use default."
func (s *Service) CreateNetwork(
	name string,
	externalIP string,
	main Plane,
	invite Plane,
) (
	*Network,
	error,
) {
	privKey, err := wireguard.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	pubKey, err := wireguard.PublicKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}

	network := &Network{
		Name:       name,
		PrivateKey: privKey,
		PublicKey:  pubKey,
		ExternalIP: externalIP,

		Main:   main,
		Invite: invite,

		Enabled:   false,
		CreatedAt: s.clock(),
	}
	if err := network.Normalize(); err != nil {
		return nil, err
	}

	_, mainNet, err := net.ParseCIDR(network.Main.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse main CIDR: %w", err)
	}
	ones, bits := mainNet.Mask.Size()
	rootCidr := &Cidr{
		Name:   name,
		Cidr:   network.Main.Cidr,
		Prefix: ones,
		Bits:   bits,
	}

	serverIP := netaddr.FirstAssignable(mainNet)
	serverRoute := netaddr.HostRoute(serverIP)
	serverCidr := &Cidr{
		Name:     "cord-server",
		Cidr:     serverRoute.String(),
		Prefix:   netaddr.TerminalPrefix(serverIP),
		Bits:     bits,
		Terminal: true,
	}
	serverPeer := &Peer{
		Name:      "cord-server",
		CidrName:  serverCidr.Name,
		Route:     serverRoute.String(),
		PublicKey: pubKey,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}

	if err := s.store.BootstrapNetwork(
		network,
		rootCidr,
		serverCidr,
		serverPeer,
	); err != nil {
		return nil, fmt.Errorf("bootstrap network: %w", mapStoreError(err))
	}

	return network, nil
}

// DeleteNetwork removes the named network and all of its resources.
// The network must be disabled first; the store enforces the guard
// atomically with the delete.
func (s *Service) DeleteNetwork(
	name string,
) error {
	if err := s.store.DeleteNetwork(name); err != nil {
		return fmt.Errorf("delete network %q: %w", name, mapStoreError(err))
	}

	return nil
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
	if err := s.store.SetNetworkEnabled(
		name,
		enabled,
	); err != nil {
		return fmt.Errorf("set network %q enabled: %w", name, mapStoreError(err))
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

// PruneNetwork removes state that has aged out of the network: expired
// unconfirmed registrations with their provisional peers, and endpoint
// sightings older than the endpoint TTL. The runtime calls it at the top
// of every reconciliation so both WireGuard peer sets are derived from
// clean state.
func (s *Service) PruneNetwork(
	network string,
) error {
	now := s.clock()

	if err := s.store.PruneExpiredRegistrations(
		network,
		now,
	); err != nil {
		return fmt.Errorf("prune expired registrations: %w", mapStoreError(err))
	}

	if err := s.store.DeleteEndpointsBefore(
		network,
		now.Add(-defaultEndpointTTL),
	); err != nil {
		return fmt.Errorf("prune endpoints: %w", mapStoreError(err))
	}

	return nil
}

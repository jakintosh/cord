package service

import (
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// Store is the persistence contract required by the server service. It
// uses domain vocabulary and domain types. The concrete SQLite
// implementation lives in internal/server/database.
type Store interface {
	// Network records.

	// GetNetwork returns the persisted network config by name.
	// Returns ErrNotFound when no matching row exists.
	GetNetwork(name string) (*NetworkConfig, error)

	// ListNetworks returns all persisted server network configs,
	// ordered by name ascending.
	ListNetworks() ([]*NetworkConfig, error)

	// ListNetworkNames returns the names of all server networks,
	// ordered by name ascending.
	ListNetworkNames() ([]string, error)

	// BootstrapNetwork atomically creates a new network together
	// with its root CIDR record, server CIDR, and initial server
	// peer. All are persisted in a single transaction. Returns
	// ErrConflict when a network with this name already exists.
	BootstrapNetwork(network *NetworkConfig, rootCidr *Cidr, serverCidr *Cidr, serverPeer *Peer) error

	// SetNetworkEnabled updates the enabled flag for a network.
	// When enabled, the daemon starts the network's WireGuard devices
	// and reconciliation loop on boot.
	SetNetworkEnabled(name string, enabled bool) error

	// DeleteNetwork removes the named network and all of its
	// resources via foreign-key cascades.
	DeleteNetwork(name string) error

	// Peer records within a network.

	// PeerExists reports whether a peer with the given name exists
	// in the network.
	PeerExists(network, name string) (bool, error)

	// GetPeer returns a peer by name within the given network.
	GetPeer(network, name string) (*Peer, error)

	// GetPeerByIP returns the confirmed, enabled peer at the given IP
	// address within the network. Used for authenticating ordinary
	// peer API calls (peers, endpoints).
	GetPeerByIP(network string, ip net.IP) (*Peer, error)

	// GetProvisionalPeerByIP returns the unconfirmed, enabled peer at
	// the given IP address within the network. Used for authenticating
	// the /confirm endpoint, which is called by peers that have
	// redeemed but not yet confirmed.
	GetProvisionalPeerByIP(network string, ip net.IP) (*Peer, error)

	// GetPeerByKey returns the peer with the given public key
	// within the network.
	GetPeerByKey(network, pubKey string) (*Peer, error)

	// ListPeers returns every peer in the network, ordered by name.
	ListPeers(network string) ([]*Peer, error)

	// InsertPeer persists a new peer in the network. Returns
	// ErrConflict when a peer with the same name, CIDR, or public key
	// already exists in this network.
	InsertPeer(network string, peer *Peer) error

	// DeletePeer removes a peer by name from the network.
	DeletePeer(network, name string) error

	// UpdatePeer applies a partial update to the named peer and
	// returns the updated record. Nil pointers mean no change.
	UpdatePeer(network, name string, update PeerUpdate) (*Peer, error)

	// CIDR records within a network.

	// GetCidr returns a CIDR by name within the network.
	GetCidr(network, name string) (*Cidr, error)

	// ListCidrs returns all CIDRs in the network.
	ListCidrs(network string) ([]*Cidr, error)

	// InsertCidr persists a new CIDR in the network. Returns
	// ErrConflict when the range overlaps an existing CIDR.
	InsertCidr(network string, cidr *Cidr) error

	// DeleteCidr removes a CIDR by name. Associated assignments
	// are also removed via foreign-key cascades.
	DeleteCidr(network, name string) error

	// UpdateCidr renames a CIDR and returns the updated record.
	UpdateCidr(network, name string, newName string) (*Cidr, error)

	// Group records within a network.

	// ListGroups returns all groups in the network.
	ListGroups(network string) ([]*Group, error)

	// InsertGroup creates a new group with the given name.
	// Returns ErrConflict when a group with this name already exists.
	InsertGroup(network, name string) (*Group, error)

	// DeleteGroup removes a group by name from the network.
	DeleteGroup(network, name string) error

	// Assignment records within a network (CIDR <-> Group).

	// ListAssignments returns all group assignments in the network.
	ListAssignments(network string) ([]*Assignment, error)

	// AssignGroup assigns a group to a CIDR.
	AssignGroup(network, cidrName, groupName string) error

	// RemoveGroup removes a group assignment from a CIDR.
	RemoveGroup(network, cidrName, groupName string) error

	// Association records within a network (Group <-> Group).

	// ListAssociations returns all group associations in the network.
	ListAssociations(network string) ([]*Association, error)

	// InsertAssociation creates an association between two groups.
	// Associations are stored normalized (group1 < group2).
	InsertAssociation(network string, a *Association) error

	// DeleteAssociation removes the association between two groups.
	DeleteAssociation(network, group1, group2 string) error

	// Topology snapshot for visibility resolution.

	// LoadTopologySnapshot loads a complete topology snapshot for
	// the given network, suitable for resolving peer visibility.
	LoadTopologySnapshot(network string) (*topology.Snapshot, error)

	// Registration records within a network.

	// GetRegistration returns a registration by name within the network.
	GetRegistration(network, name string) (*Registration, error)

	// GetRegistrationByIP returns the registration for the given
	// temporary IP within the network. Expired registrations are
	// excluded using now.
	GetRegistrationByIP(network string, ip net.IP, now time.Time) (*Registration, error)

	// ListRegistrations returns all registrations in the network
	// (active, expired, and redeemed).
	ListRegistrations(network string) ([]*Registration, error)

	// ListActiveRegistrations returns only unexpired, unconfirmed
	// registrations in the network.
	ListActiveRegistrations(network string, now time.Time) ([]*Registration, error)

	// InsertRegistration persists a new registration in the network.
	InsertRegistration(network string, reg *Registration) error

	// RedeemRegistration marks a registration as redeemed with the
	// given permanent public key. The temporary key is recorded as
	// the lookup key. now is used to check the registration has not
	// expired.
	RedeemRegistration(network string, tempPubKey, permPubKey string, now time.Time) error

	// ConfirmRegistration marks a registration as confirmed.
	ConfirmRegistration(network, name string) error

	// DeleteRegistration removes a registration by name from the
	// network.
	DeleteRegistration(network, name string) error

	// DeleteExpiredRegistrations removes all registrations whose
	// expiration time is before the given timestamp.
	DeleteExpiredRegistrations(network string, before time.Time) error

	// PruneExpiredRegistrations removes expired unconfirmed
	// registrations and any provisional peer rows whose registration
	// is gone or expired. Confirmed peers are retained; their
	// registrations are retained as audit state. Called once at the
	// top of reconcile so that both WireGuard peer sets are derived
	// from clean state.
	PruneExpiredRegistrations(network string, now time.Time) error

	// Endpoint records within a network.

	// GetRecentEndpoints returns endpoint sightings since the given
	// time, keyed by peer public key. Used for endpoint gossip.
	GetRecentEndpoints(network string, since time.Time) (map[string][]EndpointWitness, error)

	// InsertEndpointSightings persists endpoint sightings reported
	// by peers or observed by the server's WireGuard device.
	InsertEndpointSightings(network string, sightings []EndpointSighting) error

	// DeleteEndpointsBefore removes all endpoint records older than
	// the given time.
	DeleteEndpointsBefore(network string, before time.Time) error

	// Close releases the database connection.
	Close() error
}

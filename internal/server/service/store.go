package service

import (
	"net"
	"time"
)

// Store is the persistence contract required by the server service. It
// uses domain vocabulary and domain types. The concrete SQLite
// implementation lives in internal/server/database.
type Store interface {
	// Network records.

	// GetNetwork returns the persisted network record by name.
	// Returns ErrNotFound when no matching row exists.
	GetNetwork(name string) (*Network, error)

	// ListNetworks returns all persisted server network records,
	// ordered by name ascending.
	ListNetworks() ([]*Network, error)

	// BootstrapNetwork atomically creates a new network together
	// with its root CIDR record, server CIDR, and initial server
	// peer. All are persisted in a single transaction. Returns
	// ErrConflict when a network with this name already exists.
	BootstrapNetwork(network *Network, rootCidr *Cidr, serverCidr *Cidr, serverPeer *Peer) error

	// SetNetworkEnabled updates the enabled flag for a network.
	// When enabled, the daemon starts the network's WireGuard devices
	// and reconciliation loop on boot.
	SetNetworkEnabled(name string, enabled bool) error

	// DeleteNetwork removes the named network and all of its
	// resources via foreign-key cascades. The enabled guard is
	// enforced atomically with the delete: it returns
	// ErrNetworkEnabled when the network is still enabled and
	// ErrNotFound when the network does not exist.
	DeleteNetwork(name string) error

	// Peer records within a network.

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
	UpdatePeer(network, name string, update PeerDiff) (*Peer, error)

	// CIDR records within a network.

	// GetCidr returns a CIDR by name within the network.
	GetCidr(network, name string) (*Cidr, error)

	// ListCidrs returns all CIDRs in the network.
	ListCidrs(network string) ([]*Cidr, error)

	// CreateCidr persists a new CIDR contained by the network's main CIDR.
	// It returns ErrConflict when the name or range is already reserved and
	// ErrInvalidInput when the range is outside the persisted main CIDR.
	CreateCidr(network string, cidr *Cidr) error

	// UpdateCidr renames a CIDR and returns the updated record.
	UpdateCidr(network, name string, newName string) (*Cidr, error)

	// DeleteCidr removes a non-root CIDR by name. Associated assignments are
	// also removed via foreign-key cascades. It returns ErrConflict for the
	// network's root CIDR and ErrNotFound when the CIDR does not exist.
	DeleteCidr(network, name string) error

	// ListCidrGroups returns the groups directly assigned to a CIDR, ordered by
	// group name. It returns ErrNotFound when the CIDR does not exist.
	ListCidrGroups(network, cidrName string) ([]*Group, error)

	// AssignCidrGroup assigns a group to a CIDR.
	AssignCidrGroup(network, cidrName, groupName string) error

	// RemoveCidrGroup removes a group assignment from a CIDR. It returns
	// ErrNotFound when the CIDR or group does not exist. Removing an absent
	// assignment is idempotent.
	RemoveCidrGroup(network, cidrName, groupName string) error

	// Group records within a network.

	// ListGroups returns all groups in the network.
	ListGroups(network string) ([]*Group, error)

	// InsertGroup creates a new group with the given name.
	// Returns ErrConflict when a group with this name already exists.
	InsertGroup(network, name string) (*Group, error)

	// DeleteGroup removes a group by name from the network.
	DeleteGroup(network, name string) error

	// Association records within a network (Group <-> Group).

	// ListAssociations returns all group associations in the network.
	ListAssociations(network string) ([]*Association, error)

	// InsertAssociation creates an association between two groups.
	// Associations are stored normalized (group1 < group2).
	InsertAssociation(network string, a *Association) error

	// DeleteAssociation removes the association between two groups.
	DeleteAssociation(network, group1, group2 string) error

	// Network topology facts.

	// LoadTopologyState loads every CIDR, assignment, association, and managed
	// peer in one consistent read. It returns ErrNotFound when the network does
	// not exist.
	LoadTopologyState(network string) (*TopologyState, error)

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

	// CreateRegistration atomically prunes expired registrations, allocates the
	// lowest available invite route, verifies that its main route is contained
	// by the persisted main CIDR, and persists the completed registration.
	// It returns ErrConflict for a persisted name, key, or route conflict and
	// ErrRegistrationAddressExhausted when the invite network has no free route.
	// It returns ErrInvalidInput when the main route is outside the main CIDR.
	CreateRegistration(
		network string,
		params CreateRegistrationParams,
		now time.Time,
	) (*Registration, error)

	// RedeemRegistration atomically redeems an unexpired registration and
	// returns its provisional peer. Repeating redemption with the same keys is
	// idempotent while the peer remains unconfirmed. It returns
	// ErrRegistrationRedeemed when the registration was redeemed with another
	// key or its peer is already confirmed, and ErrRegistrationExpired when an
	// unredeemed registration has expired.
	RedeemRegistration(
		network string,
		tempPubKey string,
		permPubKey string,
		now time.Time,
	) (*Peer, error)

	// ConfirmPeer atomically marks a peer and its unexpired registration
	// confirmed and transfers registration group assignments to the peer's
	// terminal CIDR. now determines whether the registration has expired.
	// Confirmation is idempotent. It returns ErrRegistrationExpired when the
	// provisional peer's registration has expired.
	ConfirmPeer(network, name string, now time.Time) error

	// DeleteRegistration revokes an unconfirmed registration by name and
	// removes any provisional peer reality. Confirmed registrations are
	// immutable and return ErrConflict.
	DeleteRegistration(network, name string) error

	// PruneExpiredRegistrations removes expired unconfirmed
	// registrations and any provisional peer rows whose registration
	// is gone or expired. Confirmed peers are retained; their
	// registrations are retained as audit state. Called once at the
	// top of reconcile so that both WireGuard peer sets are derived
	// from clean state.
	PruneExpiredRegistrations(network string, now time.Time) error

	// ListRegistrationGroups returns the groups assigned to a pending
	// registration, ordered by group name. It returns ErrNotFound when the
	// registration does not exist.
	ListRegistrationGroups(network, registration string) ([]*Group, error)

	// AssignRegistrationGroup assigns a group to an unconfirmed, unexpired
	// registration. now determines whether the registration has expired.
	// It returns ErrConflict for a confirmed registration and
	// ErrRegistrationExpired for an expired registration.
	AssignRegistrationGroup(network, registration, group string, now time.Time) error

	// RemoveRegistrationGroup removes a group assignment from an unconfirmed,
	// unexpired registration. Removing an assignment that is not present is
	// idempotent. now determines whether the registration has expired. It returns
	// ErrConflict for a confirmed registration and ErrRegistrationExpired for an
	// expired registration.
	RemoveRegistrationGroup(network, registration, group string, now time.Time) error

	// Endpoint records within a network.

	// GetRecentEndpoints returns endpoint sightings since the given
	// time, keyed by peer public key. Used for endpoint gossip.
	GetRecentEndpoints(network string, since time.Time) (map[string][]EndpointWitness, error)

	// InsertEndpointSightings atomically persists endpoint sightings reported
	// by peers or observed by the server's WireGuard device. It returns
	// ErrNotFound when the network or any referenced peer does not exist and
	// leaves the entire batch unchanged.
	InsertEndpointSightings(network string, sightings []EndpointSighting) error

	// DeleteEndpointsBefore removes all endpoint records older than
	// the given time.
	DeleteEndpointsBefore(network string, before time.Time) error

	// Close releases the database connection.
	Close() error
}

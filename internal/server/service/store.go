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

	// ListNetworkNames returns the names of all server networks,
	// ordered by name ascending.
	ListNetworkNames() ([]string, error)

	// BootstrapNetwork atomically creates a new network together
	// with its root CIDR record and initial server peer. All three
	// are persisted in a single transaction. Returns ErrConflict
	// when a network with this name already exists.
	BootstrapNetwork(network *Network, rootCidr *Cidr, serverPeer *Peer) error

	// SetNetworkEnabled updates the enabled flag for a network.
	// When enabled, the daemon starts the network's WireGuard devices
	// and reconciliation loop on boot.
	SetNetworkEnabled(name string, enabled bool) error

	// DeleteNetwork removes the named network and all of its
	// resources via foreign-key cascades.
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

	// GetPeerByKey returns the peer with the given public key
	// within the network.
	GetPeerByKey(network, pubKey string) (*Peer, error)

	// ListPeers returns every peer in the network, ordered by name.
	ListPeers(network string) ([]*Peer, error)

	// InsertPeer persists a new peer in the network. Returns
	// ErrConflict when a peer with the same name, IP, or public key
	// already exists in this network.
	InsertPeer(network string, peer *Peer) error

	// DeletePeer removes a peer by name from the network.
	DeletePeer(network, name string) error

	// UpdatePeer applies a partial update to the named peer and
	// returns the updated record. Nil pointers mean no change.
	UpdatePeer(network, name string, newName *string, admin *bool, enabled *bool, confirmed *bool) (*Peer, error)

	// PeerExists reports whether a peer with the given name exists
	// in the network.
	PeerExists(network, name string) (bool, error)

	// CIDR records within a network.

	// GetCidr returns a CIDR by name within the network.
	GetCidr(network, name string) (*Cidr, error)

	// ListCidrs returns all CIDRs in the network.
	ListCidrs(network string) ([]*Cidr, error)

	// InsertCidr persists a new CIDR in the network. Returns
	// ErrConflict when the range overlaps an existing CIDR.
	InsertCidr(network string, cidr *Cidr) error

	// DeleteCidr removes a CIDR by name. Associated associations
	// are also removed via foreign-key cascades.
	DeleteCidr(network, name string) error

	// UpdateCidr renames a CIDR and returns the updated record.
	UpdateCidr(network, name string, newName string) (*Cidr, error)

	// Association records within a network.

	// ListAssociations returns all associations in the network.
	ListAssociations(network string) ([]*Association, error)

	// InsertAssociation creates an association between two CIDRs.
	// Associations are stored normalized (cidr1 < cidr2).
	InsertAssociation(network string, a *Association) error

	// DeleteAssociation removes the association between two CIDRs.
	DeleteAssociation(network, cidr1, cidr2 string) error

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
	// registrations are retained as audit state. Called from
	// buildMainPeers and buildInvitePeers so that both WireGuard
	// peer sets are derived from clean state.
	PruneExpiredRegistrations(network string, now time.Time) error

	// Endpoint records within a network.

	// GetRecentEndpoints returns endpoint sightings since the given
	// time, keyed by peer public key. Used for endpoint gossip.
	GetRecentEndpoints(network string, since time.Time) (map[string][]EndpointWitness, error)

	// InsertEndpointSightings persists endpoint sightings reported
	// by peers.
	InsertEndpointSightings(network string, sightings []EndpointSighting) error

	// DeleteEndpointsBefore removes all endpoint records older than
	// the given time.
	DeleteEndpointsBefore(network string, before time.Time) error

	// Close releases the database connection.
	Close() error
}

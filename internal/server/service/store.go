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

	// GetPeerByIP returns the confirmed peer at the given IP
	// address within the network. Unconfirmed peers are excluded.
	GetPeerByIP(network string, ip net.IP) (*Peer, error)

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
	// returns the updated record. Nil fields mean no change.
	UpdatePeer(network, name string, req UpdatePeerRequest) (*Peer, error)

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
	UpdateCidr(network, name string, req UpdateCidrRequest) (*Cidr, error)

	// Association records within a network.

	// ListAssociations returns all associations in the network.
	ListAssociations(network string) ([]*Association, error)

	// InsertAssociation creates an association between two CIDRs.
	// Associations are stored normalized (cidr1 < cidr2).
	InsertAssociation(network string, a *Association) error

	// DeleteAssociation removes the association between two CIDRs.
	DeleteAssociation(network, cidr1, cidr2 string) error

	// Invite records within a network.

	// GetInvite returns an invite by name within the network.
	GetInvite(network, name string) (*Invite, error)

	// GetInviteByIP returns the invite for the given temporary IP
	// within the network. Expired invites are excluded using now.
	GetInviteByIP(network string, ip net.IP, now time.Time) (*Invite, error)

	// ListInvites returns all invites in the network (active,
	// expired, and redeemed).
	ListInvites(network string) ([]*Invite, error)

	// ListActiveInvites returns only unredeemed, unexpired invites
	// in the network.
	ListActiveInvites(network string, now time.Time) ([]*Invite, error)

	// InsertInvite persists a new invite in the network.
	InsertInvite(network string, invite *Invite) error

	// RedeemInvite marks an invite as redeemed with the given
	// permanent public key. The temporary key is recorded as the
	// lookup key. now is used to check the invite has not expired.
	RedeemInvite(network string, tempPubKey, permPubKey string, now time.Time) error

	// DeleteInvite removes an invite by name from the network.
	DeleteInvite(network, name string) error

	// DeleteExpiredInvites removes all invites whose expiration
	// time is before the given timestamp.
	DeleteExpiredInvites(network string, before time.Time) error

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

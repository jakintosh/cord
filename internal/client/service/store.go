package service

// Store is the persistence contract required by the client service.
// It uses domain vocabulary and domain types. The concrete SQLite
// implementation lives in internal/client/database.
type Store interface {
	// Network records.

	// GetNetwork returns the persisted network record by name.
	GetNetwork(name string) (*Network, error)

	// ListNetworkNames returns the names of all installed networks,
	// ordered by name ascending.
	ListNetworkNames() ([]string, error)

	// InsertNetwork persists a new network record. Returns
	// ErrConflict when a network with this name already exists.
	InsertNetwork(network *Network) error

	// DeleteNetwork removes the named network and all of its peer
	// cache entries via foreign-key cascade.
	DeleteNetwork(name string) error

	// UpdateNetwork applies a partial update to the named network
	// and returns the updated record. Nil pointers in the request
	// mean "no change."
	UpdateNetwork(name string, req UpdateNetworkRequest) (*Network, error)

	// Peer cache within a network.

	// ReconcilePeers replaces the stored peer set for the named
	// network with the provided peers. Peers already present are
	// upserted by public key; peers not in the provided list are
	// deleted. When upserting, a locally-observed endpoint is
	// preserved if its timestamp is newer than the incoming value.
	ReconcilePeers(network string, peers []Peer) error

	// ListPeers returns all cached peers for the named network,
	// ordered by name ascending.
	ListPeers(network string) ([]*Peer, error)

	// UpdatePeerEndpoint records a locally-observed endpoint for a
	// peer identified by public key within the named network.
	UpdatePeerEndpoint(network, pubKey, endpoint string, when int64) error

	// Close releases the database connection.
	Close() error
}

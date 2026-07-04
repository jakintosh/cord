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

	// SetNetworkRedeemed transitions a network to the redeemed
	// state, recording the main network parameters returned by
	// the server's /redeem endpoint.
	SetNetworkRedeemed(name string, assignedRoute string, serverPubkey string, serverEndpoint string, serverRoute string, serverAPIPort uint16) error

	// SetNetworkConfirmed transitions a network to the confirmed
	// state and clears the temporary install scratch fields.
	SetNetworkConfirmed(name string) error

	// DeleteNetwork removes the named network and all of its peer
	// cache entries via foreign-key cascade.
	DeleteNetwork(name string) error

	// SetNetworkEnabled updates the enabled flag for a network.
	SetNetworkEnabled(name string, enabled bool) error

	// Peer cache within a network.

	// SetPeers replaces the stored peer set for the named network
	// with the provided peers. Peers already present are upserted by
	// public key; peers not in the provided list are deleted.
	SetPeers(network string, peers []Peer) error

	// ListPeers returns all cached peers for the named network,
	// ordered by name ascending. Each peer's Endpoint field is
	// populated with the best known endpoint from the endpoint
	// table (most recently observed by the server, then locally).
	ListPeers(network string) ([]*Peer, error)

	// Endpoint catalog within a network.

	// SetPeerEndpoints replaces the known endpoints for a peer
	// identified by public key. Existing endpoints not in the list
	// are deleted. Incoming endpoints are upserted;
	// server_observed_at is updated only if the incoming value is
	// newer.
	SetPeerEndpoints(network, pubKey string, endpoints []PeerEndpoint) error

	// UpdatePeerEndpointLocal sets local_observed_at on the
	// matching endpoint row. No-op if the endpoint doesn't exist.
	UpdatePeerEndpointLocal(network, pubKey, endpoint string, when int64) error

	// ListPeerEndpoints returns all known endpoints for a peer,
	// ordered by server_observed_at DESC, local_observed_at DESC.
	ListPeerEndpoints(network, pubKey string) ([]PeerEndpoint, error)

	// Close releases the database connection.
	Close() error
}

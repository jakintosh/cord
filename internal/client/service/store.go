package service

// Store is the persistence contract required by the client service.
// It uses domain vocabulary and domain types. The concrete SQLite
// implementation lives in internal/client/database.
type Store interface {
	// Install records (transient, consumed at confirm).

	InsertInstall(install *Install) error
	GetInstall(name string) (*Install, error)
	ListInstalls() ([]*Install, error)
	SetInstallRedeemed(name string, assignedRoute string, server ServerInfo) error
	DeleteInstall(name string) error

	// ConfirmInstall inserts a NetworkConfig and deletes the matching
	// Install row in a single transaction.
	ConfirmInstall(name string, nc *NetworkConfig) error

	// Network records (permanent membership).

	GetNetwork(name string) (*NetworkConfig, error)
	ListNetworkNames() ([]string, error)
	InsertNetwork(nc *NetworkConfig) error
	DeleteNetwork(name string) error
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

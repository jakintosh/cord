package service

// Store is the persistence contract required by the client service.
// It uses domain vocabulary and domain types. The concrete SQLite
// implementation lives in internal/client/database.
type Store interface {
	// Install records (transient, consumed at confirm).

	InsertInstall(install *Install) error
	GetInstall(name string) (*Install, error)
	ListInstalls() ([]*Install, error)
	RedeemInstall(name string, assignedRoute string, server ServerInfo) error
	DeleteInstall(name string) error

	// ConfirmInstall inserts a NetworkConfig and deletes the matching
	// Install row in a single transaction.
	ConfirmInstall(name string, nc *NetworkConfig) error

	// Network records (permanent membership).

	GetNetwork(name string) (*NetworkConfig, error)
	ListNetworkNames() ([]string, error)
	ListNetworks() ([]*NetworkConfig, error)
	InsertNetwork(nc *NetworkConfig) error
	DeleteNetwork(name string) error
	SetNetworkEnabled(name string, enabled bool) error
	UpdateNetwork(name string, update NetworkOptions) error

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

	// SetPeerEndpoints merges server-observed endpoints for a peer
	// identified by public key. Locally observed endpoints are retained.
	// Incoming endpoints are upserted;
	// server_observed_at is updated only if the incoming value is
	// newer.
	SetPeerEndpoints(network, pubKey string, endpoints []PeerEndpoint) error

	// UpdatePeerEndpointLocal upserts a locally observed endpoint and sets
	// local_observed_at.
	UpdatePeerEndpointLocal(network, pubKey, endpoint string, when int64) error

	// MarkPeerEndpointAttempt sets last_attempted_at on the matching
	// endpoint row. No-op if the endpoint doesn't exist.
	MarkPeerEndpointAttempt(network, pubKey, endpoint string, when int64) error

	// ListPeerEndpoints returns all known endpoints for a peer,
	// ordered by server_observed_at DESC, local_observed_at DESC.
	ListPeerEndpoints(network, pubKey string) ([]PeerEndpoint, error)

	// ListLocalEndpointsSince returns endpoints across all peers of
	// the named network with local_observed_at at or after since.
	ListLocalEndpointsSince(network string, since int64) ([]EndpointSighting, error)

	// DeletePeerEndpointsBefore removes endpoint candidates that have not been
	// observed by either the server or this client since before.
	DeletePeerEndpointsBefore(network string, before int64) error

	// Close releases the database connection.
	Close() error
}

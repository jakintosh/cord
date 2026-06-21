package service

// Store is the persistence contract required by the service.
// The concrete SQLite implementation lives in internal/client/database.
type Store interface {
	GetNetwork(name string) (*Network, error)
	ListNetworkNames() ([]string, error)
	InsertNetwork(network *Network) error
	DeleteNetwork(name string) error
	UpdateNetwork(name string, req UpdateNetworkRequest) (*Network, error)

	ReconcilePeers(network string, peers []Peer) error
	ListPeers(network string) ([]*Peer, error)
	UpdatePeerEndpoint(network, pubKey, endpoint string, when int64) error

	Close() error
}

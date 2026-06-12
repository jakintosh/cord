package client

import (
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// LocalPeer is the client's record of a network peer: identity plus
// the most recently known endpoint.
type LocalPeer struct {
	Name         string
	PublicKey    string
	Cidr         string
	Endpoint     string
	EndpointTime int64
}

// PeerStore is the client's durable peer cache for one network. It is
// implemented by the database layer and injected at construction.
type PeerStore interface {
	// ReconcilePeers replaces the stored peer set with the server's
	// view, keeping locally observed endpoints when they are fresher
	// than what the server reports.
	ReconcilePeers(peers []server.PublicPeer) error

	// ListPeers returns all stored peers, ordered by name.
	ListPeers() ([]LocalPeer, error)

	// UpdateEndpoint records a locally observed peer endpoint.
	UpdateEndpoint(publicKey string, endpoint string, when int64) error

	// Close releases the store.
	Close() error

	// Delete closes the store and removes its durable state.
	// Idempotent: deleting an absent store succeeds.
	Delete() error
}

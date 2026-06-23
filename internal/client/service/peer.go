package service

// Peer is a cached peer record stored in the client's local database.
// It represents another participant on the network as seen from this
// client. Peers are fetched from the server during sync and reconciled
// into the local cache.
type Peer struct {
	Name         string
	PublicKey    string
	Cidr         string // e.g. "10.42.0.5/16"
	Endpoint     string // last known public UDP endpoint, "host:port"
	EndpointTime int64  // unix timestamp of the last endpoint observation
}

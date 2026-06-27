package service

import "git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"

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

// peersFromDTOs converts the server's visible peer list into local Peer
// records. For each peer, the most recent endpoint witness is selected.
func peersFromDTOs(
	dtos []serverapi.VisiblePeerDTO,
) []Peer {
	peers := make([]Peer, len(dtos))
	for i, dto := range dtos {
		var endpoint string
		var endpointTime int64
		for _, ep := range dto.Endpoints {
			t := ep.Timestamp.Unix()
			if t > endpointTime {
				endpointTime = t
				endpoint = ep.Endpoint
			}
		}
		peers[i] = Peer{
			Name:         dto.Name,
			PublicKey:    dto.PublicKey,
			Cidr:         dto.Cidr,
			Endpoint:     endpoint,
			EndpointTime: endpointTime,
		}
	}
	return peers
}

// ListPeers returns all cached peers for the named network.
func (s *Service) ListPeers(
	network string,
) (
	[]*Peer,
	error,
) {
	return s.store.ListPeers(network)
}

package service

import "git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"

// Peer is a cached peer record stored in the client's local database.
// It represents another participant on the network as seen from this
// client. Peers are fetched from the server during sync and reconciled
// into the local cache.
type Peer struct {
	Name      string
	PublicKey string
	Route     string // explicit route for AllowedIPs, e.g. "10.42.0.5/32"
	Endpoint  string // best known endpoint, populated by ListPeers
}

// PeerEndpoint is a single endpoint observation for a peer. Each row
// records an endpoint address, when it was observed — either by the
// server (gossip from other peers) or locally (direct contact) — and
// when it was last tried as a rotation candidate for a stale peer.
type PeerEndpoint struct {
	Endpoint         string
	ServerObservedAt int64
	LocalObservedAt  int64
	LastAttemptedAt  int64
}

// EndpointSighting is a locally observed peer endpoint, reported to
// the server for gossip distribution.
type EndpointSighting struct {
	PeerKey  string
	Endpoint string
}

// peersFromDTOs converts the server's visible peer list into local Peer
// records.
func peersFromDTOs(
	dtos []serverapi.VisiblePeerDTO,
) []Peer {
	peers := make([]Peer, len(dtos))
	for i, dto := range dtos {
		peers[i] = Peer{
			Name:      dto.Name,
			PublicKey: dto.PublicKey,
			Route:     dto.Route,
		}
	}
	return peers
}

// endpointsFromDTOs extracts all endpoint observations for a single
// peer from its DTO.
func endpointsFromDTO(
	dto serverapi.VisiblePeerDTO,
) []PeerEndpoint {
	eps := make([]PeerEndpoint, len(dto.Endpoints))
	for i, ep := range dto.Endpoints {
		eps[i] = PeerEndpoint{
			Endpoint:         ep.Endpoint,
			ServerObservedAt: ep.Timestamp.Unix(),
		}
	}
	return eps
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

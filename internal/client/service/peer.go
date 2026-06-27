package service

import "git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"

// Peer is a cached peer record stored in the client's local database.
// It represents another participant on the network as seen from this
// client. Peers are fetched from the server during sync and reconciled
// into the local cache.
type Peer struct {
	Name      string
	PublicKey string
	Cidr      string // e.g. "10.42.0.5/16"
	Endpoint  string // best known endpoint, populated by ListPeers
}

// PeerEndpoint is a single endpoint observation for a peer. Each row
// records an endpoint address and when it was observed — either by
// the server (gossip from other peers) or locally (direct contact).
type PeerEndpoint struct {
	Endpoint         string
	ServerObservedAt int64
	LocalObservedAt  int64
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
			Cidr:      dto.Cidr,
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

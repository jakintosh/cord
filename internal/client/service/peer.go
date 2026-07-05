package service

import (
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

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

// endpointsFromProtocol extracts all endpoint observations for a single
// visible peer.
func endpointsFromProtocol(
	vp protocol.VisiblePeer,
) []PeerEndpoint {
	eps := make([]PeerEndpoint, len(vp.Endpoints))
	for i, ep := range vp.Endpoints {
		eps[i] = PeerEndpoint{
			Endpoint:         ep.Endpoint,
			ServerObservedAt: ep.Timestamp.Unix(),
		}
	}
	return eps
}

// EndpointSighting is a locally observed peer endpoint, reported to
// the server for gossip distribution. It is stored in and read back from
// the local database as domain state; the report path converts it to the
// protocol wire shape at the network boundary.
type EndpointSighting struct {
	PeerKey  string
	Endpoint string
}

func (es *EndpointSighting) toProtocol() protocol.EndpointSighting {
	return protocol.EndpointSighting{
		PeerKey:  es.PeerKey,
		Endpoint: es.Endpoint,
	}
}

func sightingsToProtocol(
	sightings []EndpointSighting,
) []protocol.EndpointSighting {
	protoSightings := make([]protocol.EndpointSighting, len(sightings))
	for i, s := range sightings {
		protoSightings[i] = s.toProtocol()
	}
	return protoSightings
}

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

// peersFromProtocol converts the server's visible peer list into local
// Peer records.
func peersFromProtocol(
	visible []protocol.VisiblePeer,
) []Peer {
	peers := make([]Peer, len(visible))
	for i, vp := range visible {
		peers[i] = Peer{
			Name:      vp.Name,
			PublicKey: vp.PublicKey,
			Route:     vp.Route,
		}
	}
	return peers
}

// PeerStatus is a cached peer joined with its live WireGuard device
// state, for operator-facing display. Runtime fields are zero-valued
// when the network isn't currently running.
type PeerStatus struct {
	Name          string
	Route         string
	Endpoint      string
	LastHandshake time.Time
	Connected     bool
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

// ListPeerStatus returns the cached peers for the named network joined
// with live device state (endpoint, last handshake, connected). If the
// network exists but isn't running, cached peers are returned with
// zero-valued runtime fields rather than an error. Returns
// ErrNetworkNotInstalled/ErrNotFound if the network doesn't exist at
// all, consistent with GetNetwork.
func (s *Service) ListPeerStatus(
	network string,
) (
	[]PeerStatus,
	error,
) {
	if _, err := s.store.GetNetwork(network); err != nil {
		return nil, err
	}

	peers, err := s.store.ListPeers(network)
	if err != nil {
		return nil, err
	}

	live := make(map[string]wireguard.PeerStatus)
	s.mu.Lock()
	n, running := s.running[network]
	s.mu.Unlock()
	if running {
		devicePeers, err := n.tunnel.device.Peers()
		if err != nil {
			return nil, err
		}
		for _, dp := range devicePeers {
			live[dp.PublicKey.String()] = dp
		}
	}

	now := s.clock()
	statuses := make([]PeerStatus, len(peers))
	for i, p := range peers {
		status := PeerStatus{
			Name:  p.Name,
			Route: p.Route,
		}
		if dp, ok := live[p.PublicKey]; ok {
			if dp.Endpoint != nil {
				status.Endpoint = dp.Endpoint.String()
			}
			status.LastHandshake = dp.LastHandshake
			status.Connected = !dp.LastHandshake.IsZero() &&
				now.Sub(dp.LastHandshake) < StaleThreshold
		}
		statuses[i] = status
	}
	return statuses, nil
}

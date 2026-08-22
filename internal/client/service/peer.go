package service

import (
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

// PeerEndpoint is a single endpoint observation for a peer. Each row
// records an endpoint address, when it was observed — either by the
// server (gossip from other peers) or locally (direct contact) — and
// when it was last tried as a rotation candidate for a stale peer.
type PeerEndpoint struct {
	Endpoint         string
	ServerObservedAt time.Time
	LocalObservedAt  time.Time
	LastAttemptedAt  time.Time
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
			ServerObservedAt: ep.Timestamp,
		}
	}
	return eps
}

// EndpointSighting is a locally observed peer endpoint, reported to
// the server for gossip distribution. It is stored in and read back from
// the local database as domain state; the runtime converts it to the
// protocol wire shape at the network boundary.
type EndpointSighting struct {
	PeerKey  string
	Endpoint string
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

// PeerObservation is one peer and its server-observed endpoints reported
// during reconciliation.
type PeerObservation struct {
	Peer      Peer
	Endpoints []PeerEndpoint
}

// peersFromProtocol converts the server snapshot's visible peer list into a
// local reconciliation input.
func peersFromProtocol(
	visible []protocol.VisiblePeer,
) []PeerObservation {
	peers := make([]PeerObservation, len(visible))
	for i, vp := range visible {
		peers[i] = PeerObservation{
			Peer: Peer{
				Name:      vp.Name,
				PublicKey: vp.PublicKey,
				Route:     vp.Route,
			},
			Endpoints: endpointsFromProtocol(vp),
		}
	}
	return peers
}

// ListPeers returns all cached peers for the named network, each with
// the best endpoint currently known for it.
func (s *Service) ListPeers(
	network string,
) (
	[]*Peer,
	error,
) {
	return s.store.ListPeers(network)
}

// ListPeerEndpoints returns every known endpoint for one peer, in
// best-first catalog order. The runtime rotates through them when a peer
// goes stale.
func (s *Service) ListPeerEndpoints(
	network string,
	pubKey string,
) (
	[]PeerEndpoint,
	error,
) {
	return s.store.ListPeerEndpoints(network, pubKey)
}

// ListLocalEndpoints returns the endpoints this client observed itself
// at or after since, across every peer of the network.
func (s *Service) ListLocalEndpoints(
	network string,
	since time.Time,
) (
	[]EndpointSighting,
	error,
) {
	return s.store.ListLocalEndpointsSince(network, since)
}

// RecordLocalEndpoint records an endpoint this client saw a peer use.
func (s *Service) RecordLocalEndpoint(
	network string,
	pubKey string,
	endpoint string,
	observedAt time.Time,
) error {
	return s.store.RecordLocalEndpoint(network, pubKey, endpoint, observedAt)
}

// RecordEndpointAttempt records that a peer's device endpoint was
// pointed at a candidate, so rotation resumes where it left off across
// daemon restarts.
func (s *Service) RecordEndpointAttempt(
	network string,
	pubKey string,
	endpoint string,
	attemptedAt time.Time,
) error {
	return s.store.RecordEndpointAttempt(network, pubKey, endpoint, attemptedAt)
}

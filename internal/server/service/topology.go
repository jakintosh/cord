package service

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// VisibleNetworkSnapshot is one peer's complete server view at a point in
// time. Peers and topology are derived from the same topology snapshot.
type VisibleNetworkSnapshot struct {
	GeneratedAt time.Time
	Peers       []*VisiblePeer
	Topology    topology.View
}

// GetNetworkTopology returns the complete topology for server-side
// administration.
func (s *Service) GetNetworkTopology(
	network string,
) (
	topology.View,
	error,
) {
	if _, err := s.store.GetNetwork(network); err != nil {
		return topology.View{}, fmt.Errorf("get network %q: %w", network, mapStoreError(err))
	}
	snapshot, err := s.store.LoadTopologySnapshot(network)
	if err != nil {
		return topology.View{}, fmt.Errorf("load topology snapshot: %w", mapStoreError(err))
	}
	compiled, err := topology.New(snapshot)
	if err != nil {
		return topology.View{}, fmt.Errorf("compile topology: %w", err)
	}
	return compiled.FullView(), nil
}

// GetVisibleNetworkSnapshot returns one peer's visible peers, endpoint
// observations, and projected topology from one consistent topology snapshot.
func (s *Service) GetVisibleNetworkSnapshot(
	network string,
	peerName string,
) (
	*VisibleNetworkSnapshot,
	error,
) {
	generatedAt := s.clock()
	snapshot, err := s.store.LoadTopologySnapshot(network)
	if err != nil {
		return nil, fmt.Errorf("load topology snapshot: %w", mapStoreError(err))
	}
	compiled, err := topology.New(snapshot)
	if err != nil {
		return nil, fmt.Errorf("compile topology: %w", err)
	}
	visibility, err := compiled.Resolver().Visibility(peerName)
	if err != nil {
		return nil, fmt.Errorf("resolve visibility for %q: %w", peerName, err)
	}
	view, err := compiled.ProjectedViewFromVisibility(visibility)
	if err != nil {
		return nil, fmt.Errorf("project topology for %q: %w", peerName, err)
	}

	recentEndpoints, err := s.store.GetRecentEndpoints(
		network,
		generatedAt.Add(-defaultEndpointTTL),
	)
	if err != nil {
		return nil, fmt.Errorf("get recent endpoints: %w", err)
	}

	peers := make([]*VisiblePeer, 0, len(visibility.Peers))
	for _, peer := range visibility.Peers {
		// Clients pin the server peer configuration from redemption data.
		if peer.Name == "cord-server" {
			continue
		}
		witnesses := recentEndpoints[peer.PublicKey]
		endpoints := make([]EndpointWitness, len(witnesses))
		for i, witness := range witnesses {
			endpoints[i] = EndpointWitness{
				Witness:   witness.Witness,
				Endpoint:  witness.Endpoint,
				Timestamp: witness.Timestamp,
			}
		}
		peers = append(peers, &VisiblePeer{
			Name:      peer.Name,
			Route:     peer.Route,
			PublicKey: peer.PublicKey,
			Endpoints: endpoints,
		})
	}

	return &VisibleNetworkSnapshot{
		GeneratedAt: generatedAt,
		Peers:       peers,
		Topology:    view,
	}, nil
}

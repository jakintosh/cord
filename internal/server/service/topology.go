package service

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// VisibleNetworkSnapshot is one peer's complete server view at a point in
// time. Peers and topology are derived from the same topology state.
type VisibleNetworkSnapshot struct {
	GeneratedAt time.Time
	Peers       []*VisiblePeer
	Topology    topology.View
}

// TopologyState is one consistent read of a network's persisted topology facts
// and managed peers. Service policy determines which peers are included when
// compiling it for administration or visibility resolution.
type TopologyState struct {
	Cidrs        []topology.Cidr
	Assignments  map[string][]string
	Associations map[string]map[string]bool
	Peers        []*Peer
}

// CompileAllPeers compiles the topology with every managed peer.
func (s *TopologyState) CompileAllPeers() (
	*topology.Topology,
	error,
) {
	return s.compile(func(*Peer) bool { return true })
}

// CompileActivePeers compiles the topology with only confirmed, enabled peers
// suitable for visibility resolution and endpoint gossip.
func (s *TopologyState) CompileActivePeers() (
	*topology.Topology,
	error,
) {
	return s.compile(func(peer *Peer) bool {
		return peer.Confirmed && peer.Enabled
	})
}

func (s *TopologyState) compile(
	includePeer func(*Peer) bool,
) (
	*topology.Topology,
	error,
) {
	peerCidr := make(map[string]string)
	peerInfo := make(map[string]topology.Peer)
	for _, peer := range s.Peers {
		if !includePeer(peer) {
			continue
		}
		peerCidr[peer.Name] = peer.CidrName
		peerInfo[peer.Name] = topology.Peer{
			Name:      peer.Name,
			PublicKey: peer.PublicKey,
			Route:     peer.Route,
		}
	}

	return topology.New(&topology.Snapshot{
		Cidrs:        s.Cidrs,
		Assignments:  s.Assignments,
		Associations: s.Associations,
		PeerCidr:     peerCidr,
		PeerInfo:     peerInfo,
	})
}

// GetNetworkTopology returns the complete topology for server-side
// administration.
func (s *Service) GetNetworkTopology(
	network string,
) (
	topology.View,
	error,
) {
	state, err := s.store.LoadTopologyState(network)
	if err != nil {
		return topology.View{}, fmt.Errorf("load topology state: %w", mapStoreError(err))
	}

	compiled, err := state.CompileAllPeers()
	if err != nil {
		return topology.View{}, fmt.Errorf("compile topology: %w", err)
	}

	return compiled.FullView(), nil
}

// GetVisibleNetworkSnapshot returns one peer's visible peers, endpoint
// observations, and projected topology from one consistent topology state.
func (s *Service) GetVisibleNetworkSnapshot(
	network string,
	peerName string,
) (
	*VisibleNetworkSnapshot,
	error,
) {
	generatedAt := s.clock()
	state, err := s.store.LoadTopologyState(network)
	if err != nil {
		return nil, fmt.Errorf("load topology state: %w", mapStoreError(err))
	}

	compiled, err := state.CompileActivePeers()
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

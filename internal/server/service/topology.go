package service

import (
	"fmt"
	"sync"
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
	model, err := s.getNetworkModel(network)
	if err != nil {
		return topology.View{}, fmt.Errorf("get network topology: %w", err)
	}

	return model.fullTopology.FullView(), nil
}

// GetVisibleNetworkSnapshot returns one peer's visible peers, endpoint
// observations, and projected topology. The subject view comes from the
// network's compiled model; endpoint observations are loaded fresh.
func (s *Service) GetVisibleNetworkSnapshot(
	network string,
	peerName string,
) (
	*VisibleNetworkSnapshot,
	error,
) {
	generatedAt := s.clock()

	model, err := s.getNetworkModel(network)
	if err != nil {
		return nil, err
	}

	view, err := model.peerViews.view(model.activeTopology, peerName)
	if err != nil {
		return nil, err
	}

	recentEndpoints, err := s.store.GetRecentEndpoints(
		network,
		generatedAt.Add(-defaultEndpointTTL),
	)
	if err != nil {
		return nil, fmt.Errorf("get recent endpoints: %w", err)
	}

	peers := make([]*VisiblePeer, 0, len(view.visibility.Peers))
	for _, peer := range view.visibility.Peers {
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
		Topology:    view.projected,
	}, nil
}

// peerView is one subject's visibility explanation and projected
// topology within a network model.
type peerView struct {
	visibility *topology.Visibility
	projected  topology.View
}

// peerViewCache retains subject views for the lifetime of one network
// model. Views are computed lazily and never evicted; invalidation
// replaces the whole model.
type peerViewCache struct {
	mu    sync.Mutex
	views map[string]*peerView
}

// view returns the subject's cached view, computing it once on first
// use. Lookup errors are not cached.
func (c *peerViewCache) view(
	active *topology.Topology,
	peerName string,
) (
	*peerView,
	error,
) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if view, ok := c.views[peerName]; ok {
		return view, nil
	}

	visibility, err := active.Resolver().Visibility(peerName)
	if err != nil {
		return nil, fmt.Errorf("resolve visibility for %q: %w", peerName, err)
	}

	projected, err := active.ProjectedViewFromVisibility(visibility)
	if err != nil {
		return nil, fmt.Errorf("project topology for %q: %w", peerName, err)
	}

	view := &peerView{
		visibility: &visibility,
		projected:  projected,
	}
	c.views[peerName] = view

	return view, nil
}

// networkModel is the immutable compiled model of one network,
// constructed from a single consistent read of its topology state. It
// is safe for concurrent readers; the subject view cache is the only
// lazily filled part and guards itself.
type networkModel struct {
	fullTopology   *topology.Topology
	activeTopology *topology.Topology

	activeByName map[string]*Peer
	activeByIP   map[string]*Peer

	peerViews peerViewCache
}

// newNetworkModel compiles a network model from one topology state.
func newNetworkModel(
	state *TopologyState,
) (
	*networkModel,
	error,
) {
	fullTopology, err := state.CompileAllPeers()
	if err != nil {
		return nil, fmt.Errorf("compile full topology: %w", err)
	}

	activeTopology, err := state.CompileActivePeers()
	if err != nil {
		return nil, fmt.Errorf("compile active topology: %w", err)
	}

	activeByName := make(map[string]*Peer, len(state.Peers))
	activeByIP := make(map[string]*Peer, len(state.Peers))
	for _, peer := range state.Peers {
		if !peer.Confirmed || !peer.Enabled {
			continue
		}
		activeByName[peer.Name] = peer
		activeByIP[peer.Route] = peer
	}

	return &networkModel{
		fullTopology:   fullTopology,
		activeTopology: activeTopology,
		activeByName:   activeByName,
		activeByIP:     activeByIP,
		peerViews:      peerViewCache{views: make(map[string]*peerView)},
	}, nil
}

// getNetworkModel returns the network's compiled model, loading and
// publishing it on first use. A per-network generation guard prevents
// an in-flight load from publishing after a mutation invalidated it.
func (s *Service) getNetworkModel(
	network string,
) (
	*networkModel,
	error,
) {
	for {
		s.mu.Lock()
		model, ok := s.models[network]
		generation := s.generations[network]
		s.mu.Unlock()
		if ok {
			return model, nil
		}

		state, err := s.store.LoadTopologyState(network)
		if err != nil {
			return nil, fmt.Errorf("load topology state: %w", mapStoreError(err))
		}

		loaded, err := newNetworkModel(state)
		if err != nil {
			return nil, err
		}

		s.mu.Lock()
		cached, ok := s.models[network]
		stale := s.generations[network] != generation
		if !ok && !stale {
			s.models[network] = loaded
		}
		s.mu.Unlock()

		if ok {
			return cached, nil
		}
		if !stale {
			return loaded, nil
		}
		// A mutation invalidated the network while this load was in
		// flight; discard it and read the mutated state.
	}
}

// invalidateModel drops the network's compiled model. Mutations call
// it synchronously once the store accepts the change; the next reader
// rebuilds from the mutated state.
func (s *Service) invalidateModel(
	network string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The generation advances even for removed networks so an
	// in-flight load cannot publish stale state into a recreated
	// network of the same name.
	s.generations[network]++
	delete(s.models, network)
}

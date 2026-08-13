package peer

import (
	"cmp"
	"slices"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// toVisiblePeer collapses a domain VisiblePeer's per-witness endpoint
// observations into the wire shape: one entry per endpoint, keeping the
// newest sighting timestamp.
func toVisiblePeer(
	p *service.VisiblePeer,
) protocol.VisiblePeer {
	seen := map[string]time.Time{}
	for _, e := range p.Endpoints {
		if prev, ok := seen[e.Endpoint]; !ok || e.Timestamp.After(prev) {
			seen[e.Endpoint] = e.Timestamp
		}
	}

	endpoints := make([]protocol.EndpointWitness, 0, len(seen))
	for ep, ts := range seen {
		endpoints = append(endpoints, protocol.EndpointWitness{
			Endpoint:  ep,
			Timestamp: ts,
		})
	}

	slices.SortFunc(endpoints, func(a, b protocol.EndpointWitness) int {
		if n := cmp.Compare(a.Endpoint, b.Endpoint); n != 0 {
			return n
		}
		return a.Timestamp.Compare(b.Timestamp)
	})

	return protocol.VisiblePeer{
		Name:      p.Name,
		Route:     p.Route,
		PublicKey: p.PublicKey,
		Endpoints: endpoints,
	}
}

func toVisibleNetworkSnapshot(
	snapshot *service.VisibleNetworkSnapshot,
) protocol.VisibleNetworkSnapshot {
	peers := make([]protocol.VisiblePeer, len(snapshot.Peers))
	for i, peer := range snapshot.Peers {
		peers[i] = toVisiblePeer(peer)
	}
	return protocol.VisibleNetworkSnapshot{
		GeneratedAt: snapshot.GeneratedAt,
		Peers:       peers,
		Topology:    topologyToProtocol(snapshot.Topology),
	}
}

func topologyToProtocol(
	view topology.View,
) protocol.TopologyView {
	nodes := make([]protocol.TopologyNode, len(view.Nodes))
	for i, node := range view.Nodes {
		nodes[i] = protocol.TopologyNode{
			Name:          node.Cidr.Name,
			CIDR:          node.Cidr.Cidr,
			Terminal:      node.Cidr.Terminal,
			DisplayParent: node.DisplayParent,
			Groups:        node.Groups,
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
	}
	associations := make([]protocol.TopologyAssociation, len(view.Associations))
	for i, association := range view.Associations {
		associations[i] = protocol.TopologyAssociation{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}
	return protocol.TopologyView{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: view.EffectiveGroups,
		SubjectPeer:     view.SubjectPeer,
	}
}

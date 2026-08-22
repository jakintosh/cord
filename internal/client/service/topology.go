package service

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

// CachedTopology is the last complete server projection stored locally.
type CachedTopology struct {
	View        topology.View
	GeneratedAt time.Time
	SyncedAt    time.Time
}

// NetworkReconciliation is one complete server synchronization result. Peer
// and topology state must be persisted atomically.
type NetworkReconciliation struct {
	Peers       []PeerObservation
	Topology    topology.View
	GeneratedAt time.Time
	ReceivedAt  time.Time
	PruneBefore time.Time
}

func networkReconciliationFromProtocol(
	snapshot protocol.VisibleNetworkSnapshot,
	receivedAt time.Time,
	pruneBefore time.Time,
) (
	NetworkReconciliation,
	error,
) {
	view, err := topologyFromProtocol(snapshot.Topology)
	if err != nil {
		return NetworkReconciliation{}, fmt.Errorf("decode topology: %w", err)
	}
	return NetworkReconciliation{
		Peers:       peersFromProtocol(snapshot.Peers),
		Topology:    view,
		GeneratedAt: snapshot.GeneratedAt,
		ReceivedAt:  receivedAt,
		PruneBefore: pruneBefore,
	}, nil
}

// ApplyNetworkSnapshot validates one complete server view and persists
// it as the network's cached peers and topology. It is the durable half
// of a sync: the runtime fetches the snapshot over the tunnel and
// applies the resulting peer set to the device, but every conversion
// from the wire shape and every store write happens here.
func (s *Service) ApplyNetworkSnapshot(
	network string,
	snapshot protocol.VisibleNetworkSnapshot,
) error {
	now := s.clock()

	reconciliation, err := networkReconciliationFromProtocol(
		snapshot,
		now,
		now.Add(-EndpointTTL),
	)
	if err != nil {
		return fmt.Errorf("%w: validate network snapshot: %v", ErrInvalidInput, err)
	}

	if err := s.store.ApplyNetworkReconciliation(network, reconciliation); err != nil {
		return fmt.Errorf("apply network reconciliation: %w", err)
	}
	return nil
}

func topologyFromProtocol(
	view protocol.TopologyView,
) (
	topology.View,
	error,
) {
	nodes := make([]topology.ViewNode, len(view.Nodes))
	for i, node := range view.Nodes {
		cidr, err := topology.CidrFromString(node.Name, node.CIDR, node.Terminal)
		if err != nil {
			return topology.View{}, err
		}
		nodes[i] = topology.ViewNode{
			Cidr:          cidr,
			DisplayParent: node.DisplayParent,
			Groups:        node.Groups,
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
	}
	associations := make([]topology.Association, len(view.Associations))
	for i, association := range view.Associations {
		associations[i] = topology.Association{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}
	return topology.NormalizeView(topology.View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: view.EffectiveGroups,
		SubjectPeer:     view.SubjectPeer,
	})
}

// GetNetworkTopology returns the last successfully synchronized topology. It
// remains available while the network is disabled or offline.
func (s *Service) GetNetworkTopology(
	network string,
) (
	*CachedTopology,
	error,
) {
	return s.store.GetNetworkTopology(network)
}

package api

import (
	"context"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

type NetworkTopology struct {
	GeneratedAt     time.Time             `json:"generated_at"`
	SyncedAt        time.Time             `json:"synced_at"`
	Nodes           []TopologyNode        `json:"nodes"`
	Associations    []TopologyAssociation `json:"associations"`
	EffectiveGroups []string              `json:"effective_groups"`
	SubjectPeer     string                `json:"subject_peer"`
}

type TopologyNode struct {
	Name          string   `json:"name"`
	CIDR          string   `json:"cidr"`
	Terminal      bool     `json:"terminal"`
	DisplayParent string   `json:"display_parent,omitempty"`
	Groups        []string `json:"groups"`
	PeerName      string   `json:"peer_name,omitempty"`
	Subject       bool     `json:"subject"`
}

type TopologyAssociation struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

func topologyFromService(
	cached *service.CachedTopology,
) NetworkTopology {
	nodes := make([]TopologyNode, len(cached.View.Nodes))
	for i, node := range cached.View.Nodes {
		nodes[i] = TopologyNode{
			Name:          node.Cidr.Name,
			CIDR:          node.Cidr.Cidr,
			Terminal:      node.Cidr.Terminal,
			DisplayParent: node.DisplayParent,
			Groups:        node.Groups,
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
	}

	associations := make([]TopologyAssociation, len(cached.View.Associations))
	for i, association := range cached.View.Associations {
		associations[i] = TopologyAssociation{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}

	return NetworkTopology{
		GeneratedAt:     cached.GeneratedAt,
		SyncedAt:        cached.SyncedAt,
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: cached.View.EffectiveGroups,
		SubjectPeer:     cached.View.SubjectPeer,
	}
}

func (t NetworkTopology) ToView() (
	topology.View,
	error,
) {
	nodes := make([]topology.ViewNode, len(t.Nodes))
	for i, node := range t.Nodes {
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
	associations := make([]topology.Association, len(t.Associations))
	for i, association := range t.Associations {
		associations[i] = topology.Association{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}
	return topology.NormalizeView(topology.View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: t.EffectiveGroups,
		SubjectPeer:     t.SubjectPeer,
	})
}

func (a *API) handleGetNetworkTopology(
	w http.ResponseWriter,
	r *http.Request,
) {
	networkName := r.PathValue("name")

	cached, err := a.service.GetNetworkTopology(networkName)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, topologyFromService(cached))
}

func (c *Client) GetNetworkTopology(
	ctx context.Context,
	network string,
) (
	NetworkTopology,
	error,
) {
	var result NetworkTopology
	return result, c.wire.Get(ctx, "/networks/"+network+"/topology", &result)
}

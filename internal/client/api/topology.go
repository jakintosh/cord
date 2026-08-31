package api

import (
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
)

func (a *API) handleGetNetworkTopology(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	result, err := a.runtime.GetNetworkTopology(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	topology := topologyFromRuntime(result)
	wire.WriteData(w, http.StatusOK, topology)
}

func topologyFromRuntime(
	t runtime.NetworkTopology,
) NetworkTopology {
	nodes := make([]TopologyNode, len(t.View.Nodes))
	for i, node := range t.View.Nodes {
		nodes[i] = TopologyNode{
			Name:          node.Cidr.Name,
			CIDR:          node.Cidr.Cidr,
			Terminal:      node.Cidr.Terminal,
			DisplayParent: node.DisplayParent,
			Groups:        node.Groups,
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
		if node.PeerName != "" {
			if state, ok := t.Connected[node.PeerName]; ok {
				nodes[i].Connected = &state
			}
		}
	}

	associations := make([]TopologyAssociation, len(t.View.Associations))
	for i, association := range t.View.Associations {
		associations[i] = TopologyAssociation{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}

	return NetworkTopology{
		GeneratedAt:     t.GeneratedAt,
		SyncedAt:        t.SyncedAt,
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: t.View.EffectiveGroups,
		SubjectPeer:     t.View.SubjectPeer,
	}
}

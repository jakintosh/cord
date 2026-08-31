package server

import "context"

// NetworkTopology is a server-managed network's complete topology.
type NetworkTopology struct {
	Nodes           []TopologyNode        `json:"nodes"`
	Associations    []TopologyAssociation `json:"associations"`
	EffectiveGroups []string              `json:"effective_groups"`
	SubjectPeer     string                `json:"subject_peer"`
}

// TopologyNode is one node in a topology response.
type TopologyNode struct {
	Name          string   `json:"name"`
	CIDR          string   `json:"cidr"`
	Terminal      bool     `json:"terminal"`
	DisplayParent string   `json:"display_parent,omitempty"`
	Groups        []string `json:"groups"`
	PeerName      string   `json:"peer_name,omitempty"`
	Subject       bool     `json:"subject"`
	Connected     *bool    `json:"connected,omitempty"`
}

// TopologyAssociation connects two groups in a topology response.
type TopologyAssociation struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

// GetNetworkTopology returns a network's topology.
func (c *Client) GetNetworkTopology(
	ctx context.Context,
	network string,
) (
	NetworkTopology,
	error,
) {
	var result NetworkTopology
	return result, c.wire.Get(ctx, "/networks/"+segment(network)+"/topology", &result)
}

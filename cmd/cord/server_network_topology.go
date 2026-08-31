package main

import (
	"os"

	"git.sr.ht/~jakintosh/command-go/pkg/args"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	topotext "git.studiopollinator.com/pollinator/cord/internal/topology/text"
	adminserver "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

var serverNetworkTopology = &args.Command{
	Name: "topology",
	Help: "show a network's full CIDR and group topology",
	Operands: []args.Operand{
		{
			Name: "network",
			Help: "network name",
		},
	},
	Handler: func(i *args.Input) error {
		network := i.GetOperand("network")

		client, err := serverClient(i)
		if err != nil {
			return err
		}

		result, err := client.GetNetworkTopology(i.Context(), network)
		if err != nil {
			return err
		}

		if i.GetFlag("json") {
			return printJSON(result)
		}

		view, err := serverTopologyView(result)
		if err != nil {
			return err
		}

		connected := make(map[string]bool, len(result.Nodes))
		for _, node := range result.Nodes {
			if node.Connected != nil {
				connected[node.Name] = *node.Connected
			}
		}
		return topotext.Render(os.Stdout, view, topotext.Options{
			Heading:   "topology",
			Color:     terminalStyleEnabled(os.Stdout),
			Connected: connected,
		})
	},
}

func serverTopologyView(result adminserver.NetworkTopology) (topology.View, error) {
	nodes := make([]topology.ViewNode, len(result.Nodes))
	for i, node := range result.Nodes {
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
	associations := make([]topology.Association, len(result.Associations))
	for i, association := range result.Associations {
		associations[i] = topology.Association{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}
	return topology.NormalizeView(topology.View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: result.EffectiveGroups,
		SubjectPeer:     result.SubjectPeer,
	})
}

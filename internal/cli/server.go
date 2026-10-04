package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	adminserver "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

// ServerStatus presents server resources and activity outcomes.
func (c *CLI) ServerStatus(
	s adminserver.Status,
) error {
	if c.format == FormatJSON {
		return c.JSON(s)
	}
	root := serverResource(s.Snapshot)
	root.name, root.health = "server", s.Health
	var networks []savedNetwork
	for _, n := range s.Networks {
		networks = append(networks, savedNetwork{
			name:   n.Name,
			health: n.Health,
			reason: n.Reason,
			activities: map[string]statusActivity{
				"reconcile": serverActivity(n.Reconcile),
			},
		})
	}
	root.mergeNetworks(networks)
	return c.status(root, s.Version, nil)
}

// ServerTopology presents the server's complete topology and peer observations.
func (c *CLI) ServerTopology(
	result adminserver.NetworkTopology,
) error {
	if c.format == FormatJSON {
		return c.JSON(result)
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

	return c.Topology(view, TopologyOptions{
		Connected: connected,
	})
}

// ServerNetworkDetail presents a managed network's saved configuration.
func (c *CLI) ServerNetworkDetail(
	n adminserver.Network,
) error {
	if c.format == FormatJSON {
		return c.JSON(n)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "name: %s\n", render.Clean(n.Name))
	fmt.Fprintf(&out, "external_ip: %s\n", render.Clean(n.ExternalIP))
	fmt.Fprintf(&out, "main_name: %s\n", render.Clean(n.MainName))
	fmt.Fprintf(&out, "main_cidr: %s\n", render.Clean(n.MainCidr))
	fmt.Fprintf(&out, "main_wg_port: %d\n", n.MainWgPort)
	fmt.Fprintf(&out, "main_api_port: %d\n", n.MainApiPort)
	fmt.Fprintf(&out, "invite_name: %s\n", render.Clean(n.InviteName))
	fmt.Fprintf(&out, "invite_cidr: %s\n", render.Clean(n.InviteCidr))
	fmt.Fprintf(&out, "invite_wg_port: %d\n", n.InviteWgPort)
	fmt.Fprintf(&out, "invite_api_port: %d\n", n.InviteApiPort)
	fmt.Fprintf(&out, "enabled: %t\n", n.Enabled)

	_, err := io.WriteString(c.out, out.String())
	return err
}

// ServerNetworkRuntime reports degraded health after a server network change.
func (c *CLI) ServerNetworkRuntime(
	status adminserver.NetworkStatus,
) error {
	if c.format == FormatJSON {
		return c.JSON(status)
	}

	return c.networkHealth(status.Health, status.Reason)
}

// ServerPeers presents the server's managed peers.
func (c *CLI) ServerPeers(
	peers []adminserver.Peer,
) error {
	if c.format == FormatJSON {
		if peers == nil {
			peers = []adminserver.Peer{}
		}
		return c.JSON(peers)
	}
	rows := make([][]string, len(peers))
	for idx, p := range peers {
		rows[idx] = []string{
			p.Name,
			p.Route,
			strconv.FormatBool(p.Admin),
			strconv.FormatBool(p.Enabled),
			p.PublicKey,
		}
	}

	return c.table([]string{"NAME", "ROUTE", "ADMIN", "ENABLED", "PUBLIC KEY"}, rows)
}

// Groups presents the names of network groups.
func (c *CLI) Groups(
	groups []adminserver.Group,
) error {
	if c.format == FormatJSON {
		if groups == nil {
			groups = []adminserver.Group{}
		}
		return c.JSON(groups)
	}

	rows := make([][]string, len(groups))
	for idx, g := range groups {
		rows[idx] = []string{g.Name}
	}

	return c.table([]string{"NAME"}, rows)
}

// Registrations presents invitation registration state.
func (c *CLI) Registrations(
	registrations []adminserver.Registration,
) error {
	if c.format == FormatJSON {
		if registrations == nil {
			registrations = []adminserver.Registration{}
		}
		return c.JSON(registrations)
	}

	rows := make([][]string, len(registrations))
	for idx, reg := range registrations {
		rows[idx] = []string{
			reg.Name,
			reg.Route,
			strconv.FormatBool(reg.Admin),
			strconv.FormatBool(reg.Redeemed),
			reg.ExpiresAt,
		}
	}

	return c.table([]string{"NAME", "ROUTE", "ADMIN", "REDEEMED", "EXPIRES AT"}, rows)
}

// Cidrs presents named network ranges.
func (c *CLI) Cidrs(
	cidrs []adminserver.Cidr,
) error {
	if c.format == FormatJSON {
		if cidrs == nil {
			cidrs = []adminserver.Cidr{}
		}
		return c.JSON(cidrs)
	}

	rows := make([][]string, len(cidrs))
	for idx, cidr := range cidrs {
		rows[idx] = []string{cidr.Name, cidr.Cidr}
	}

	return c.table([]string{"NAME", "CIDR"}, rows)
}

// Associations presents group connections.
func (c *CLI) Associations(
	associations []adminserver.Association,
) error {
	if c.format == FormatJSON {
		if associations == nil {
			associations = []adminserver.Association{}
		}
		return c.JSON(associations)
	}

	rows := make([][]string, len(associations))
	for idx, a := range associations {
		rows[idx] = []string{a.Group1, a.Group2}
	}

	return c.table([]string{"GROUP1", "GROUP2"}, rows)
}

func serverActivity(
	s adminserver.ActivityStatus,
) statusActivity {
	return statusActivity{
		IntervalSeconds: s.IntervalSeconds,
		LastAttemptAt:   s.LastAttemptAt,
		LastSuccessAt:   s.LastSuccessAt,
		Error:           s.Error,
	}
}

func serverResource(
	s adminserver.ResourceSnapshot,
) statusResource {
	n := statusResource{
		kind:       s.Kind,
		name:       s.Name,
		state:      s.State,
		target:     s.Target,
		health:     s.Health,
		err:        s.Error,
		activities: make(map[string]statusActivity),
	}
	for name, a := range s.Activities {
		n.activities[name] = serverActivity(a)
	}
	for _, child := range s.Children {
		n.children = append(n.children, serverResource(child))
	}
	return n
}

func serverTopologyView(
	result adminserver.NetworkTopology,
) (
	topology.View,
	error,
) {
	nodes := make([]topology.ViewNode, len(result.Nodes))
	for i, node := range result.Nodes {
		cidr, err := topology.CidrFromString(
			node.Name,
			node.CIDR,
			node.Terminal,
		)
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

	return topology.View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: result.EffectiveGroups,
		SubjectPeer:     result.SubjectPeer,
	}, nil
}

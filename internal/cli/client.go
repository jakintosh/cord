package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	adminclient "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
)

// ClientStatus presents client resources, activity outcomes, and pending installs.
func (c *CLI) ClientStatus(
	s adminclient.Status,
) error {
	if c.format == FormatJSON {
		return c.JSON(s)
	}
	root := clientResource(s.Snapshot)
	root.name, root.health = "client", s.Health
	var networks []savedNetwork
	for _, n := range s.Networks {
		networks = append(networks, savedNetwork{
			name:   n.Name,
			health: n.Health,
			reason: n.Reason,
			activities: map[string]statusActivity{
				"reconcile": clientActivity(n.Reconcile),
				"sync":      clientActivity(n.Sync),
				"scan":      clientActivity(n.Scan),
				"report":    clientActivity(n.Report),
			},
		})
	}
	root.mergeNetworks(networks)
	var installs []statusInstall
	for _, i := range s.Installs {
		installs = append(installs, statusInstall{
			name:  i.Name,
			phase: i.State,
		})
	}
	return c.status(root, s.Version, installs)
}

// ClientTopology presents the client's projected topology and sync time.
func (c *CLI) ClientTopology(
	result adminclient.NetworkTopology,
) error {
	if c.format == FormatJSON {
		return c.JSON(result)
	}

	view, err := clientTopologyView(result)
	if err != nil {
		return err
	}

	connected := make(map[string]bool, len(result.Nodes))
	for _, node := range result.Nodes {
		if node.Connected != nil {
			connected[node.Name] = *node.Connected
		}
	}

	syncTime := result.SyncedAt.Format(time.RFC3339)
	syncTimeStr := render.RelativeTime(syncTime, c.clock())
	return c.Topology(view, TopologyOptions{
		Heading:     "topology (projected)",
		Metadata:    fmt.Sprintf("synced: %s", syncTimeStr),
		BoldSubject: true,
		Connected:   connected,
	})
}

// ClientNetworkDetail presents an installed network's saved configuration.
func (c *CLI) ClientNetworkDetail(
	n adminclient.Network,
) error {
	if c.format == FormatJSON {
		return c.JSON(n)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "name: %s\n", render.Clean(n.Name))
	fmt.Fprintf(&out, "state: %s\n", render.Clean(n.State))
	fmt.Fprintf(&out, "enabled: %t\n", n.Enabled)
	if n.Address != "" {
		fmt.Fprintf(&out, "address: %s\n", render.Clean(n.Address))
	}
	if n.Interface != "" {
		fmt.Fprintf(&out, "interface: %s\n", render.Clean(n.Interface))
	}
	if n.ListenPort != 0 {
		fmt.Fprintf(&out, "listen_port: %d\n", n.ListenPort)
	}
	if n.ServerEndpoint != "" {
		fmt.Fprintf(&out, "server_endpoint: %s\n", render.Clean(n.ServerEndpoint))
	}

	_, err := io.WriteString(c.out, out.String())
	return err
}

// ClientNetworkRuntime reports degraded health after a client network change.
func (c *CLI) ClientNetworkRuntime(
	status adminclient.NetworkStatus,
) error {
	if c.format == FormatJSON {
		return c.JSON(status)
	}
	return c.networkHealth(status.Health, status.Reason)
}

// ClientPeers presents cached peers and their live connection observations.
func (c *CLI) ClientPeers(
	peers []adminclient.Peer,
) error {
	if c.format == FormatJSON {
		if peers == nil {
			peers = []adminclient.Peer{}
		}
		return c.JSON(peers)
	}

	rows := make([][]string, len(peers))
	for idx, p := range peers {
		lastHandshake := ""
		if p.LastHandshake != nil {
			lastHandshake = *p.LastHandshake
		}
		rows[idx] = []string{
			p.Name,
			p.Route,
			p.Endpoint,
			render.RelativeTime(lastHandshake, c.clock()),
			strconv.FormatBool(p.Connected),
		}
	}

	return c.table([]string{"NAME", "ROUTE", "ENDPOINT", "LAST HANDSHAKE", "CONNECTED"}, rows)
}

func clientActivity(
	s adminclient.ActivityStatus,
) statusActivity {
	return statusActivity{
		NextAttemptAt:   s.NextAttemptAt,
		IntervalSeconds: s.IntervalSeconds,
		LastAttemptAt:   s.LastAttemptAt,
		LastSuccessAt:   s.LastSuccessAt,
		Error:           s.Error,
	}
}

func clientResource(
	s adminclient.ResourceSnapshot,
) statusResource {
	n := statusResource{
		kind:       s.Kind,
		name:       s.Name,
		state:      s.State,
		target:     s.Target,
		health:     s.Health,
		reason:     s.Reason,
		err:        s.Error,
		activities: make(map[string]statusActivity),
	}
	for name, a := range s.Activities {
		n.activities[name] = clientActivity(a)
	}
	for _, child := range s.Children {
		n.children = append(n.children, clientResource(child))
	}
	return n
}

func clientTopologyView(
	result adminclient.NetworkTopology,
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

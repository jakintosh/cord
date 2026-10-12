package service_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

func TestApplyNetworkSnapshot_ConvertsTopologyAndRecordsSyncTime(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	snapshot := testutil.NetworkSnapshot()
	snapshot.GeneratedAt = testutil.FixedTime.Add(-time.Minute)
	snapshot.Topology.Nodes[0].DisplayParent = "office"
	snapshot.Topology.Nodes[0].Groups = []string{"staff"}
	snapshot.Topology.Nodes = append(snapshot.Topology.Nodes, protocol.TopologyNode{
		Name: "office", CIDR: "10.42.0.0/24", Groups: []string{"office"},
	})
	snapshot.Topology.EffectiveGroups = []string{"staff", "office"}
	snapshot.Topology.Associations = []protocol.TopologyAssociation{{Group1: "staff", Group2: "office"}}

	if err := env.Service.ApplyNetworkSnapshot("testnet", snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := env.Service.GetNetworkTopology("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if !got.GeneratedAt.Equal(snapshot.GeneratedAt) || !got.SyncedAt.Equal(testutil.FixedTime) {
		t.Fatalf("generated/synced = %v / %v, want %v / %v", got.GeneratedAt, got.SyncedAt, snapshot.GeneratedAt, testutil.FixedTime)
	}
	if got.View.SubjectPeer != "self" || len(got.View.Nodes) != 2 {
		t.Fatalf("view = %#v, want subject and both nodes", got.View)
	}
	for _, node := range got.View.Nodes {
		switch node.Cidr.Name {
		case "self":
			if node.Cidr.Cidr != "10.42.0.5/32" || !node.Cidr.Terminal ||
				node.DisplayParent != "office" || node.PeerName != "self" || !node.Subject ||
				!reflect.DeepEqual(node.Groups, []string{"staff"}) {
				t.Errorf("self node = %#v", node)
			}
		case "office":
			if node.Cidr.Cidr != "10.42.0.0/24" || node.Cidr.Terminal ||
				!reflect.DeepEqual(node.Groups, []string{"office"}) {
				t.Errorf("office node = %#v", node)
			}
		default:
			t.Errorf("unexpected node %q", node.Cidr.Name)
		}
	}
	if !reflect.DeepEqual(got.View.EffectiveGroups, []string{"office", "staff"}) {
		t.Errorf("effective groups = %#v", got.View.EffectiveGroups)
	}
	if len(got.View.Associations) != 1 || got.View.Associations[0].Group1 != "office" || got.View.Associations[0].Group2 != "staff" {
		t.Errorf("associations = %#v", got.View.Associations)
	}
}

func TestApplyNetworkSnapshot_InvalidTopologyPreservesCachedState(
	t *testing.T,
) {
	for _, tc := range []struct {
		name   string
		mutate func(*protocol.TopologyView)
	}{
		{"invalid CIDR", func(v *protocol.TopologyView) { v.Nodes[0].CIDR = "invalid" }},
		{"missing parent", func(v *protocol.TopologyView) { v.Nodes[0].DisplayParent = "missing" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)
			testutil.SeedNetworkDirect(t, env.Database, "testnet")
			snapshot := testutil.NetworkSnapshot(protocol.VisiblePeer{
				Name: "alice", PublicKey: "alice-key", Route: "10.42.0.9/32",
			})
			if err := env.Service.ApplyNetworkSnapshot("testnet", snapshot); err != nil {
				t.Fatal(err)
			}
			before, err := env.Service.GetNetworkTopology("testnet")
			if err != nil {
				t.Fatal(err)
			}
			peersBefore, err := env.Service.ListPeers("testnet")
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&snapshot.Topology)
			snapshot.Peers = nil

			err = env.Service.ApplyNetworkSnapshot("testnet", snapshot)
			if !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			after, err := env.Service.GetNetworkTopology("testnet")
			if err != nil {
				t.Fatal(err)
			}
			peersAfter, err := env.Service.ListPeers("testnet")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(peersBefore, peersAfter) {
				t.Fatal("invalid snapshot changed cached topology or peers")
			}
		})
	}
}

func TestGetNetworkTopology_DistinguishesMissingNetworkAndSnapshot(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	if _, err := env.Service.GetNetworkTopology("missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("missing network = %v, want ErrNotFound", err)
	}
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if _, err := env.Service.GetNetworkTopology("testnet"); !errors.Is(err, service.ErrTopologyUnavailable) {
		t.Fatalf("missing snapshot = %v, want ErrTopologyUnavailable", err)
	}
}

func TestApplyNetworkSnapshot_MissingNetwork(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	err := env.Service.ApplyNetworkSnapshot("missing", testutil.NetworkSnapshot())
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

package service_test

import (
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func TestTopologyState_CompilesAllOrActivePeers(t *testing.T) {
	var cidrs []topology.Cidr
	for _, name := range []string{"active", "disabled", "pending"} {
		cidr, err := topology.CidrFromString(name, map[string]string{
			"active":   "10.0.0.1/32",
			"disabled": "10.0.0.2/32",
			"pending":  "10.0.0.3/32",
		}[name], true)
		if err != nil {
			t.Fatal(err)
		}
		cidrs = append(cidrs, cidr)
	}

	state := &service.TopologyState{
		Cidrs: cidrs,
		Peers: []*service.Peer{
			{Name: "active", CidrName: "active", Enabled: true, Confirmed: true},
			{Name: "disabled", CidrName: "disabled", Enabled: false, Confirmed: true},
			{Name: "pending", CidrName: "pending", Enabled: true, Confirmed: false},
		},
	}

	allPeers, err := state.CompileAllPeers()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"active", "disabled", "pending"} {
		if cidr, ok := allPeers.Resolver().GetPeerCIDR(name); !ok || cidr != name {
			t.Errorf("all-peer topology CIDR for %q = %q, %v", name, cidr, ok)
		}
	}

	activePeers, err := state.CompileActivePeers()
	if err != nil {
		t.Fatal(err)
	}
	if cidr, ok := activePeers.Resolver().GetPeerCIDR("active"); !ok || cidr != "active" {
		t.Fatalf("active peer CIDR = %q, %v", cidr, ok)
	}
	for _, name := range []string{"disabled", "pending"} {
		if _, ok := activePeers.Resolver().GetPeerCIDR(name); ok {
			t.Errorf("inactive peer %q included in active topology", name)
		}
	}
}

func TestTopologyState_PreservesDistinctPeerAndCIDRNames(t *testing.T) {
	cidr, err := topology.CidrFromString("cord-server-cidr", "10.0.0.1/32", true)
	if err != nil {
		t.Fatal(err)
	}
	state := &service.TopologyState{
		Cidrs: []topology.Cidr{cidr},
		Peers: []*service.Peer{
			{Name: "cord-server", CidrName: "cord-server-cidr"},
		},
	}

	compiled, err := state.CompileAllPeers()
	if err != nil {
		t.Fatal(err)
	}
	peerCIDR, ok := compiled.Resolver().GetPeerCIDR("cord-server")
	if !ok || peerCIDR != "cord-server-cidr" {
		t.Fatalf("peer CIDR = %q, %v, want cord-server-cidr", peerCIDR, ok)
	}
}

func TestGetNetworkTopology_IncludesDisabledAndUnconfirmedPeers(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	testutil.SeedPeerDB(t, env.Database, "testnet", "disabled", "10.0.0.40/32", "disabled-key", false, false, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "pending", "10.0.0.41/32", "pending-key", false, true, false)

	view, err := env.Service.GetNetworkTopology("testnet")
	if err != nil {
		t.Fatal(err)
	}
	peerByCIDR := make(map[string]string, len(view.Nodes))
	for _, node := range view.Nodes {
		peerByCIDR[node.Cidr.Name] = node.PeerName
	}
	for _, name := range []string{"disabled", "pending"} {
		if peerByCIDR[name] != name {
			t.Errorf("peer for CIDR %q = %q, want %q", name, peerByCIDR[name], name)
		}
	}
}

func TestGetVisibleNetworkSnapshot_ProjectsMiddleCIDRAndContractsParent(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	for _, cidr := range []struct {
		name string
		cidr string
	}{
		{name: "hidden-east", cidr: "10.0.0.0/20"},
		{name: "api", cidr: "10.0.1.0/24"},
		{name: "ops", cidr: "10.0.32.0/24"},
	} {
		if err := env.Service.CreateCidr("testnet", cidr.name, cidr.cidr); err != nil {
			t.Fatalf("create CIDR %s: %v", cidr.name, err)
		}
	}
	testutil.SeedPeerDB(t, env.Database, "testnet", "api-1", "10.0.1.1/32", "api-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "frank", "10.0.32.5/32", "frank-key", false, true, true)

	for _, group := range []string{"app:api", "team:ops"} {
		if _, err := env.Service.CreateGroup("testnet", group); err != nil {
			t.Fatalf("create group %s: %v", group, err)
		}
	}
	if err := env.Service.AssignCidrGroup("testnet", "api", "app:api"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.AssignCidrGroup("testnet", "ops", "team:ops"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.CreateAssociation("testnet", "app:api", "team:ops"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := env.Service.GetVisibleNetworkSnapshot("testnet", "frank")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Peers) != 1 || snapshot.Peers[0].Name != "api-1" {
		t.Fatalf("visible peers = %#v, want api-1", snapshot.Peers)
	}

	names := make([]string, len(snapshot.Topology.Nodes))
	parents := make(map[string]string)
	for i, node := range snapshot.Topology.Nodes {
		names[i] = node.Cidr.Name
		parents[node.Cidr.Name] = node.DisplayParent
	}
	want := []string{"testnet", "api", "api-1", "ops", "frank"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("topology nodes = %v, want %v", names, want)
	}
	if parents["api"] != "testnet" {
		t.Fatalf("api display parent = %q, want contracted testnet", parents["api"])
	}
	if parents["api-1"] != "api" {
		t.Fatalf("api-1 display parent = %q, want visible api", parents["api-1"])
	}
}

func TestGetNetworkTopology_NotFound(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	if _, err := env.Service.GetNetworkTopology("missing"); err == nil {
		t.Fatal("expected missing network error")
	}
}

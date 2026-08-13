package service_test

import (
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

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

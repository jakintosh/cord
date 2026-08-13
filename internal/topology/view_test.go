package topology

import (
	"reflect"
	"strings"
	"testing"
)

func TestCidrFromString_RejectsHostBits(t *testing.T) {
	_, err := CidrFromString("network", "10.0.0.1/24", false)
	if err == nil || !strings.Contains(err.Error(), "host bits are set") {
		t.Fatalf("CidrFromString() error = %v, want host bits error", err)
	}
}

func viewCidr(
	t *testing.T,
	name string,
	cidr string,
	terminal bool,
) Cidr {
	t.Helper()
	result, err := CidrFromString(name, cidr, terminal)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func projectionSnapshot(
	t *testing.T,
) *Snapshot {
	t.Helper()
	return &Snapshot{
		Cidrs: []Cidr{
			viewCidr(t, "self", "10.2.1.12/32", true),
			viewCidr(t, "hidden-east", "10.1.0.0/16", false),
			viewCidr(t, "root", "10.0.0.0/8", false),
			viewCidr(t, "api", "10.1.2.0/24", false),
			viewCidr(t, "api-2", "10.1.2.2/32", true),
			viewCidr(t, "west", "10.2.0.0/16", false),
			viewCidr(t, "ops", "10.2.1.0/24", false),
			viewCidr(t, "api-1", "10.1.2.1/32", true),
		},
		Assignments: map[string][]string{
			"root":        {"org:all"},
			"west":        {"region:west"},
			"ops":         {"team:ops"},
			"hidden-east": {"hidden:group"},
			"api":         {"app:api", "unrelated"},
		},
		Associations: map[string]map[string]bool{
			"team:ops": {"app:api": true, "unused": true},
			"app:api":  {"team:ops": true},
			"unused":   {"team:ops": true},
		},
		PeerCidr: map[string]string{
			"frank": "self",
			"api-1": "api-1",
			"api-2": "api-2",
		},
		PeerInfo: map[string]Peer{
			"frank": {Name: "frank", Route: "10.2.1.12/32"},
			"api-1": {Name: "api-1", Route: "10.1.2.1/32"},
			"api-2": {Name: "api-2", Route: "10.1.2.2/32"},
		},
	}
}

func TestProjectedView_ContractsHiddenParentsAndPreservesCIDROrder(
	t *testing.T,
) {
	topo, err := New(projectionSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}

	view, err := topo.ProjectedView("frank")
	if err != nil {
		t.Fatal(err)
	}

	gotNames := make([]string, len(view.Nodes))
	parents := make(map[string]string)
	for i, node := range view.Nodes {
		gotNames[i] = node.Cidr.Name
		parents[node.Cidr.Name] = node.DisplayParent
	}
	wantNames := []string{"root", "api", "api-1", "api-2", "west", "ops", "self"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("node order = %v, want %v", gotNames, wantNames)
	}
	if parents["api"] != "root" {
		t.Fatalf("api display parent = %q, want contracted root", parents["api"])
	}
	if parents["api-1"] != "api" || parents["api-2"] != "api" {
		t.Fatalf("visible child structure not preserved: %v", parents)
	}
	if gotNames[len(gotNames)-1] != "self" {
		t.Fatal("subject was promoted out of ascending CIDR order")
	}
}

func TestProjectedView_FiltersGroupsAndUnusedAssociations(
	t *testing.T,
) {
	topo, err := New(projectionSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	view, err := topo.ProjectedView("frank")
	if err != nil {
		t.Fatal(err)
	}

	var api ViewNode
	for _, node := range view.Nodes {
		if node.Cidr.Name == "api" {
			api = node
			break
		}
	}
	if !reflect.DeepEqual(api.Groups, []string{"app:api"}) {
		t.Fatalf("api groups = %v, want only causal group", api.Groups)
	}
	wantAssociations := []Association{{Group1: "app:api", Group2: "team:ops"}}
	if !reflect.DeepEqual(view.Associations, wantAssociations) {
		t.Fatalf("associations = %v, want %v", view.Associations, wantAssociations)
	}
}

func TestNormalizeView_RejectsDisplayParentCycle(
	t *testing.T,
) {
	_, err := NormalizeView(View{Nodes: []ViewNode{
		{Cidr: viewCidr(t, "one", "10.0.0.0/24", false), DisplayParent: "two"},
		{Cidr: viewCidr(t, "two", "10.0.1.0/24", false), DisplayParent: "one"},
	}})
	if err == nil {
		t.Fatal("expected display parent cycle error")
	}
}

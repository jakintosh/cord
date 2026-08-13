package topology

import (
	"net"
	"strings"
	"testing"
)

func TestBuildContainment_OrdersAndFindsNearestParents(t *testing.T) {
	index, err := buildContainment([]Cidr{
		makeCidr("leaf", "10.1.2.0/24", false),
		makeCidr("branch-b", "10.2.0.0/16", false),
		makeCidr("root", "10.0.0.0/8", false),
		makeCidr("branch-a", "10.1.0.0/16", false),
	})
	if err != nil {
		t.Fatal(err)
	}

	wantNames := []string{"root", "branch-a", "leaf", "branch-b"}
	wantParents := []int{noParent, 0, 1, 0}
	wantDepths := []int{0, 1, 2, 1}
	for i, entry := range index.entries {
		if entry.cidr.Name != wantNames[i] {
			t.Errorf("entry %d name = %q, want %q", i, entry.cidr.Name, wantNames[i])
		}
		if entry.parent != wantParents[i] {
			t.Errorf("entry %d parent = %d, want %d", i, entry.parent, wantParents[i])
		}
		if entry.depth != wantDepths[i] {
			t.Errorf("entry %d depth = %d, want %d", i, entry.depth, wantDepths[i])
		}
	}
}

func TestBuildContainment_MultipleAddressFamilies(t *testing.T) {
	index, err := buildContainment([]Cidr{
		makeCidr("v6-leaf", "fd00::1/128", true),
		makeCidr("v4-root", "10.0.0.0/8", false),
		makeCidr("v6-root", "fd00::/32", false),
		makeCidr("v4-leaf", "10.0.0.1/32", true),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"v4-root", "v6-root"} {
		entry := index.entries[index.byName[name]]
		if entry.parent != noParent || entry.depth != 0 {
			t.Errorf("%s should be a root, got parent %d depth %d", name, entry.parent, entry.depth)
		}
	}
	for child, parent := range map[string]string{
		"v4-leaf": "v4-root",
		"v6-leaf": "v6-root",
	} {
		entry := index.entries[index.byName[child]]
		parentEntry := index.entries[entry.parent]
		if parentEntry.cidr.Name != parent {
			t.Errorf("%s parent = %s, want %s", child, parentEntry.cidr.Name, parent)
		}
	}
}

func TestBuildContainment_RejectsDuplicateRange(t *testing.T) {
	_, err := buildContainment([]Cidr{
		makeCidr("one", "10.0.0.0/24", false),
		makeCidr("two", "10.0.0.0/24", false),
	})
	if err == nil || !strings.Contains(err.Error(), "same range") {
		t.Fatalf("expected duplicate range error, got %v", err)
	}
}

func TestBuildContainment_RejectsMismatchedCIDRMetadata(t *testing.T) {
	cidr := makeCidr("network", "10.0.0.0/24", false)
	cidr.Cidr = "10.0.1.0/24"

	_, err := buildContainment([]Cidr{cidr})
	if err == nil || !strings.Contains(err.Error(), "range does not match") {
		t.Fatalf("buildContainment() error = %v, want range mismatch", err)
	}
}

func TestBuildContainment_RejectsMismatchedPrefixMetadata(t *testing.T) {
	cidr := makeCidr("network", "10.0.0.0/24", false)
	cidr.Prefix = 25
	cidr.Last = net.ParseIP("10.0.0.127").To4()

	_, err := buildContainment([]Cidr{cidr})
	if err == nil || !strings.Contains(err.Error(), "metadata does not match") {
		t.Fatalf("buildContainment() error = %v, want metadata mismatch", err)
	}
}

func TestNewTopology_SharesContainmentResults(t *testing.T) {
	topo, err := New(&Snapshot{
		Cidrs: []Cidr{
			makeCidr("leaf", "10.0.1.1/32", true),
			makeCidr("root", "10.0.0.0/16", false),
		},
		Assignments: map[string][]string{
			"root": {"org"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	leaf := topo.Tree().Node("leaf")
	if leaf.Parent == nil || leaf.Parent.Cidr.Name != "root" {
		t.Fatal("compiled tree did not use derived parent")
	}
	groups, err := topo.Resolver().GetEffectiveGroups("leaf")
	if err != nil {
		t.Fatal(err)
	}
	if !groups["org"] {
		t.Fatal("compiled resolver did not inherit through derived parent")
	}
}

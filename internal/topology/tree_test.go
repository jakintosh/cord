package topology

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func mustNewTree(t *testing.T, s *Snapshot) *Tree {
	t.Helper()
	tree, err := NewTree(s)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestNewTree_RootOnly(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
		},
		Assignments: map[string][]string{},
	}
	tr := mustNewTree(t, s)
	node := tr.Node("root")
	if node == nil {
		t.Fatal("root node not found")
	}
	if node.Parent != nil {
		t.Error("root should have no parent")
	}
	if node.Depth != 0 {
		t.Errorf("root depth should be 0, got %d", node.Depth)
	}
	if len(tr.Nodes()) != 1 {
		t.Errorf("expected 1 node, got %d", len(tr.Nodes()))
	}
}

func TestNewTree_ParentChild(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("child", "10.0.1.0/24", false),
		},
		Assignments: map[string][]string{},
	}
	tr := mustNewTree(t, s)
	root := tr.Node("root")
	child := tr.Node("child")
	if root == nil || child == nil {
		t.Fatal("nodes not found")
	}
	if child.Parent != root {
		t.Error("child parent should be root")
	}
	if root.Depth != 0 {
		t.Errorf("root depth should be 0, got %d", root.Depth)
	}
	if child.Depth != 1 {
		t.Errorf("child depth should be 1, got %d", child.Depth)
	}
	if len(root.Children) != 1 || root.Children[0] != child {
		t.Error("root should have child in Children")
	}
}

func TestNewTree_DeepNesting(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("mid", "10.0.1.0/24", false),
			makeCidr("leaf", "10.0.1.1/32", true),
		},
		Assignments: map[string][]string{},
	}
	tr := mustNewTree(t, s)
	root := tr.Node("root")
	mid := tr.Node("mid")
	leaf := tr.Node("leaf")
	if root.Depth != 0 {
		t.Errorf("root depth = %d, want 0", root.Depth)
	}
	if mid.Depth != 1 {
		t.Errorf("mid depth = %d, want 1", mid.Depth)
	}
	if leaf.Depth != 2 {
		t.Errorf("leaf depth = %d, want 2", leaf.Depth)
	}
	if leaf.Parent != mid {
		t.Error("leaf parent should be mid")
	}
	if mid.Parent != root {
		t.Error("mid parent should be root")
	}
}

func TestNewTree_GroupsAttached(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("leaf", "10.0.1.1/32", true),
		},
		Assignments: map[string][]string{
			"root": {"org"},
			"leaf": {"app:web"},
		},
	}
	tr := mustNewTree(t, s)
	root := tr.Node("root")
	leaf := tr.Node("leaf")
	if !reflect.DeepEqual(root.Groups, []string{"org"}) {
		t.Errorf("root groups = %v, want [org]", root.Groups)
	}
	if !reflect.DeepEqual(leaf.Groups, []string{"app:web"}) {
		t.Errorf("leaf groups = %v, want [app:web]", leaf.Groups)
	}
}

func TestNewTree_IPv6(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("v6root", "fd00::/32", false),
			makeCidr("v6sub", "fd00:0:1::/64", false),
			makeCidr("v6leaf", "fd00:0:1::1/128", true),
		},
		Assignments: map[string][]string{},
	}
	tr := mustNewTree(t, s)
	root := tr.Node("v6root")
	sub := tr.Node("v6sub")
	leaf := tr.Node("v6leaf")
	if root == nil || sub == nil || leaf == nil {
		t.Fatal("nodes not found")
	}
	if leaf.Parent != sub {
		t.Fatal("leaf parent should be sub")
	}
	if sub.Parent != root {
		t.Fatal("sub parent should be root")
	}
}

func TestTree_PrintAncestry(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("mid", "10.0.1.0/24", false),
			makeCidr("leaf", "10.0.1.1/32", true),
		},
		Assignments: map[string][]string{
			"root": {"org"},
			"mid":  {"engineering"},
			"leaf": {"app:web"},
		},
	}
	tr := mustNewTree(t, s)
	var buf bytes.Buffer
	tr.PrintAncestry(&buf, "leaf")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "10.0.0.0/16") || !strings.Contains(lines[0], "=> root") {
		t.Errorf("line 0 should show root: %s", lines[0])
	}
	if !strings.Contains(lines[0], "[org]") {
		t.Errorf("line 0 should show [org] group: %s", lines[0])
	}
	if !strings.Contains(lines[1], "10.0.1.0/24") || !strings.Contains(lines[1], "=> mid") {
		t.Errorf("line 1 should show mid: %s", lines[1])
	}
	if !strings.Contains(lines[1], "[engineering]") {
		t.Errorf("line 1 should show [engineering] group: %s", lines[1])
	}
	if !strings.Contains(lines[2], "10.0.1.1/32") || !strings.Contains(lines[2], "=> leaf") {
		t.Errorf("line 2 should show leaf: %s", lines[2])
	}
	if !strings.Contains(lines[2], "[app:web]") {
		t.Errorf("line 2 should show [app:web] group: %s", lines[2])
	}

	if !strings.HasPrefix(lines[0], "10.0.0.0/16") {
		t.Errorf("root should have no indent prefix: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "- 10.0.1.0/24") {
		t.Errorf("mid should have '- ' prefix: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "-- 10.0.1.1/32") {
		t.Errorf("leaf should have '-- ' prefix: %q", lines[2])
	}
}

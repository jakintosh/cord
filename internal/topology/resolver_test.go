package topology

import "testing"

func makePeerInfo(
	name string,
	key string,
	route string,
) Peer {
	return Peer{
		Name:      name,
		PublicKey: key,
		Route:     route,
	}
}

func contains(
	set map[string]bool,
	key string,
) bool {
	return set[key]
}

func names(peers []Peer) []string {
	n := make([]string, len(peers))
	for i, p := range peers {
		n[i] = p.Name
	}
	return n
}

func TestNewResolver_Invalid(t *testing.T) {
	_, err := NewResolver(&Snapshot{})
	if err != nil {
		t.Fatal("empty &snapshot should be valid")
	}
}

func TestEffectiveGroups_NoInheritance(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("leaf", "10.0.1.1/32", true),
		},
		Assignments: map[string][]string{
			"root": {"users"},
			"leaf": {"app:web"},
		},
		Associations: map[string]map[string]bool{},
		PeerCidr:     map[string]string{},
		PeerInfo:     map[string]Peer{},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	leafGroups, _ := r.GetEffectiveGroups("leaf")
	if !contains(leafGroups, "users") {
		t.Error("leaf should inherit 'users' from root")
	}
	if !contains(leafGroups, "app:web") {
		t.Error("leaf should have direct group 'app:web'")
	}
}

func TestEffectiveGroups_MultiLevelInheritance(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("subnet", "10.0.1.0/24", false),
			makeCidr("leaf", "10.0.1.1/32", true),
		},
		Assignments: map[string][]string{
			"root":   {"org"},
			"subnet": {"engineering"},
			"leaf":   {},
		},
		Associations: map[string]map[string]bool{},
		PeerCidr:     map[string]string{},
		PeerInfo:     map[string]Peer{},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	leafGroups, _ := r.GetEffectiveGroups("leaf")
	if !contains(leafGroups, "org") {
		t.Error("leaf should inherit 'org' from root via subnet")
	}
	if !contains(leafGroups, "engineering") {
		t.Error("leaf should inherit 'engineering' from subnet")
	}
}

func TestEffectiveGroups_SiblingIndependence(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("eng", "10.0.1.0/24", false),
			makeCidr("sales", "10.0.2.0/24", false),
			makeCidr("alice", "10.0.1.1/32", true),
			makeCidr("bob", "10.0.2.1/32", true),
		},
		Assignments: map[string][]string{
			"root":  {"org"},
			"eng":   {"engineering"},
			"sales": {"sales-team"},
		},
		Associations: map[string]map[string]bool{},
		PeerCidr:     map[string]string{},
		PeerInfo:     map[string]Peer{},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	aliceGroups, _ := r.GetEffectiveGroups("alice")
	bobGroups, _ := r.GetEffectiveGroups("bob")
	if !contains(aliceGroups, "engineering") {
		t.Error("alice should have engineering")
	}
	if contains(aliceGroups, "sales-team") {
		t.Error("alice should NOT have sales-team")
	}
	if !contains(bobGroups, "sales-team") {
		t.Error("bob should have sales-team")
	}
	if contains(bobGroups, "engineering") {
		t.Error("bob should NOT have engineering")
	}
}

func TestVisiblePeers_SelfAssociation(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("eng", "10.0.1.0/24", false),
			makeCidr("alice", "10.0.1.1/32", true),
			makeCidr("bob", "10.0.1.2/32", true),
		},
		Assignments: map[string][]string{
			"eng": {"engineering"},
		},
		Associations: map[string]map[string]bool{
			"engineering": {"engineering": true},
		},
		PeerCidr: map[string]string{
			"alice": "alice",
			"bob":   "bob",
		},
		PeerInfo: map[string]Peer{
			"alice": makePeerInfo("alice", "key-a", "10.0.1.1/32"),
			"bob":   makePeerInfo("bob", "key-b", "10.0.1.2/32"),
		},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := r.VisiblePeers("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible peer, got %d", len(visible))
	}
	if visible[0].Name != "bob" {
		t.Errorf("expected bob, got %s", visible[0].Name)
	}
}

func TestVisiblePeers_NoSelfAssociation_Unidirectional(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("servers", "10.0.1.0/24", false),
			makeCidr("users", "10.0.2.0/24", false),
			makeCidr("svr", "10.0.1.1/32", true),
			makeCidr("usr", "10.0.2.1/32", true),
		},
		Assignments: map[string][]string{
			"servers": {"app:server"},
			"users":   {"app:user"},
		},
		Associations: map[string]map[string]bool{
			"app:server": {"app:user": true},
			"app:user":   {"app:server": true},
		},
		PeerCidr: map[string]string{
			"svr": "svr",
			"usr": "usr",
		},
		PeerInfo: map[string]Peer{
			"svr": makePeerInfo("svr", "key-s", "10.0.1.1/32"),
			"usr": makePeerInfo("usr", "key-u", "10.0.2.1/32"),
		},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	visible, _ := r.VisiblePeers("svr")
	if len(visible) != 1 || visible[0].Name != "usr" {
		t.Errorf("svr should see usr, got %v", names(visible))
	}
	visible, _ = r.VisiblePeers("usr")
	if len(visible) != 1 || visible[0].Name != "svr" {
		t.Errorf("usr should see svr, got %v", names(visible))
	}
}

func TestVisiblePeers_NoGroupMembership_NoVisibility(t *testing.T) {
	s := &Snapshot{
		Cidrs: []Cidr{
			makeCidr("root", "10.0.0.0/16", false),
			makeCidr("a", "10.0.1.1/32", true),
			makeCidr("b", "10.0.2.1/32", true),
		},
		Assignments: map[string][]string{
			"a": {"group-a"},
			"b": {"group-b"},
		},
		Associations: map[string]map[string]bool{},
		PeerCidr: map[string]string{
			"alice": "a",
			"bob":   "b",
		},
		PeerInfo: map[string]Peer{
			"alice": makePeerInfo("alice", "key-a", "10.0.1.1/32"),
			"bob":   makePeerInfo("bob", "key-b", "10.0.2.1/32"),
		},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	visible, _ := r.VisiblePeers("alice")
	if len(visible) != 0 {
		t.Errorf("alice should see nobody (no associations), got %v", names(visible))
	}
}

package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedTopologyNetwork(t *testing.T, db *database.DB) {
	t.Helper()

	name := "toponet"
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          name,
				Cidr:          "10.50.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          name + "-i",
				Cidr:          "10.51.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: time.Now(),
		},
		&service.Cidr{
			Name:   "toponet",
			Cidr:   "10.50.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server",
			Cidr:     "10.50.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server",
			Route:     "10.50.0.1/32",
			PublicKey: "pub-server",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}

	for _, c := range []service.Cidr{
		{
			Name:   "subnet-a",
			Cidr:   "10.50.1.0/24",
			Prefix: 24,
			Bits:   32,
		},
		{
			Name:   "subnet-b",
			Cidr:   "10.50.2.0/24",
			Prefix: 24,
			Bits:   32,
		},
		{
			Name:     "alice",
			Cidr:     "10.50.1.5/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		{
			Name:     "bob",
			Cidr:     "10.50.2.5/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
	} {
		if err := db.CreateCidr("toponet", &c); err != nil {
			t.Fatalf("seed cidr %s: %v", c.Name, err)
		}
	}

	for _, g := range []string{"devops", "platform"} {
		if _, err := db.InsertGroup("toponet", g); err != nil {
			t.Fatalf("seed group %s: %v", g, err)
		}
	}

	for _, pair := range [][2]string{
		{"subnet-a", "devops"},
		{"alice", "platform"},
	} {
		if err := db.AssignCidrGroup("toponet", pair[0], pair[1]); err != nil {
			t.Fatalf("assign %s -> %s: %v", pair[0], pair[1], err)
		}
	}

	if err := db.InsertAssociation("toponet", &service.Association{
		Group1: "devops",
		Group2: "platform",
	}); err != nil {
		t.Fatalf("seed association: %v", err)
	}

	for _, p := range []service.Peer{
		{
			Name:      "alice",
			CidrName:  "alice",
			Route:     "10.50.1.5/32",
			PublicKey: "pub-alice",
			Admin:     false,
			Enabled:   true,
			Confirmed: true,
		},
		{
			Name:      "bob",
			CidrName:  "bob",
			Route:     "10.50.2.5/32",
			PublicKey: "pub-bob",
			Admin:     false,
			Enabled:   true,
			Confirmed: true,
		},
	} {
		if err := db.InsertPeer("toponet", &p); err != nil {
			t.Fatalf("seed peer %s: %v", p.Name, err)
		}
	}
}

func TestLoadTopologyState_Cidrs(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	state, err := db.LoadTopologyState("toponet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Cidrs) != 6 {
		t.Fatalf("expected 6 cidrs, got %d", len(state.Cidrs))
	}

	byName := make(map[string]bool)
	for _, c := range state.Cidrs {
		byName[c.Name] = true
	}
	for _, want := range []string{"toponet", "cord-server", "subnet-a", "subnet-b", "alice", "bob"} {
		if !byName[want] {
			t.Errorf("missing cidr %q", want)
		}
	}

	for _, c := range state.Cidrs {
		if c.Name == "cord-server" {
			if !c.Terminal {
				t.Error("cord-server should be terminal")
			}
			if c.Cidr != "10.50.0.1/32" {
				t.Errorf("cord-server cidr = %q, want 10.50.0.1/32", c.Cidr)
			}
			if c.Prefix != 32 {
				t.Errorf("cord-server prefix = %d, want 32", c.Prefix)
			}
			if c.Bits != 32 {
				t.Errorf("cord-server bits = %d, want 32", c.Bits)
			}
		}
		if c.Name == "toponet" {
			if c.Terminal {
				t.Error("toponet should not be terminal")
			}
			if c.Prefix != 16 {
				t.Errorf("toponet prefix = %d, want 16", c.Prefix)
			}
		}
	}
}

func TestLoadTopologyState_Assignments(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	state, err := db.LoadTopologyState("toponet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Assignments) == 0 {
		t.Fatal("expected assignments, got none")
	}

	groups := state.Assignments["subnet-a"]
	if len(groups) != 1 || groups[0] != "devops" {
		t.Errorf("subnet-a assignments = %v, want [devops]", groups)
	}

	groups = state.Assignments["alice"]
	if len(groups) != 1 || groups[0] != "platform" {
		t.Errorf("alice assignments = %v, want [platform]", groups)
	}

	if _, ok := state.Assignments["subnet-b"]; ok {
		t.Error("subnet-b should have no assignments")
	}
}

func TestLoadTopologyState_Associations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	state, err := db.LoadTopologyState("toponet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Associations) != 2 {
		t.Fatalf("expected 2 groups in associations, got %d", len(state.Associations))
	}

	if !state.Associations["devops"]["platform"] {
		t.Error("devops->platform should be associated")
	}
	if !state.Associations["platform"]["devops"] {
		t.Error("platform->devops should be associated (symmetric)")
	}
}

func TestLoadTopologyState_Peers(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	state, err := db.LoadTopologyState("toponet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Peers) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(state.Peers))
	}

	byName := make(map[string]*service.Peer, len(state.Peers))
	for _, peer := range state.Peers {
		byName[peer.Name] = peer
	}

	info := byName["alice"]
	if info == nil {
		t.Fatal("missing alice peer")
	}
	if info.Name != "alice" {
		t.Errorf("name = %q, want alice", info.Name)
	}
	if info.CidrName != "alice" {
		t.Errorf("cidr name = %q, want alice", info.CidrName)
	}
	if info.PublicKey != "pub-alice" {
		t.Errorf("public_key = %q, want pub-alice", info.PublicKey)
	}
	if info.Route != "10.50.1.5/32" {
		t.Errorf("route = %q, want 10.50.1.5/32", info.Route)
	}
}

func TestLoadTopologyState_IncludesManagedPeerStates(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)
	testutil.SeedPeerDB(
		t,
		db,
		"toponet",
		"pending",
		"10.50.1.7/32",
		"pub-pending",
		false,
		true,
		false,
	)
	testutil.SeedPeerDB(
		t,
		db,
		"toponet",
		"disabled",
		"10.50.1.8/32",
		"pub-disabled",
		false,
		false,
		true,
	)

	state, err := db.LoadTopologyState("toponet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}
	byName := make(map[string]*service.Peer, len(state.Peers))
	for _, peer := range state.Peers {
		byName[peer.Name] = peer
	}
	if pending := byName["pending"]; pending == nil || pending.Confirmed || !pending.Enabled {
		t.Errorf("pending peer = %+v, want enabled and unconfirmed", pending)
	}
	if disabled := byName["disabled"]; disabled == nil || !disabled.Confirmed || disabled.Enabled {
		t.Errorf("disabled peer = %+v, want confirmed and disabled", disabled)
	}
}

func TestLoadTopologyState_EmptyNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	name := "emptynet"
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          name,
				Cidr:          "10.60.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          name + "-i",
				Cidr:          "10.61.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: time.Now(),
		},
		&service.Cidr{
			Name:   "emptynet",
			Cidr:   "10.60.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server",
			Cidr:     "10.60.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server",
			Route:     "10.60.0.1/32",
			PublicKey: "pub-server",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}

	state, err := db.LoadTopologyState("emptynet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Cidrs) != 2 {
		t.Fatalf("expected 2 cidrs (root + server), got %d", len(state.Cidrs))
	}

	if len(state.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(state.Peers))
	}
	if state.Peers[0].Name != "cord-server" {
		t.Error("missing cord-server peer")
	}

	if len(state.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(state.Assignments))
	}
	if len(state.Associations) != 0 {
		t.Errorf("expected 0 associations, got %d", len(state.Associations))
	}
}

func TestLoadTopologyState_MultipleAssociations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	if _, err := db.InsertGroup("toponet", "security"); err != nil {
		t.Fatalf("seed group security: %v", err)
	}
	for _, pair := range [][2]string{
		{"security", "devops"},
		{"security", "platform"},
	} {
		if err := db.InsertAssociation("toponet", &service.Association{
			Group1: pair[0],
			Group2: pair[1],
		}); err != nil {
			t.Fatalf("insert association %s-%s: %v", pair[0], pair[1], err)
		}
	}

	state, err := db.LoadTopologyState("toponet")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Associations) != 3 {
		t.Fatalf("expected 3 groups in associations, got %d", len(state.Associations))
	}

	if !state.Associations["security"]["devops"] {
		t.Error("security->devops should be associated")
	}
	if !state.Associations["security"]["platform"] {
		t.Error("security->platform should be associated")
	}
	if !state.Associations["devops"]["security"] {
		t.Error("devops->security should be associated (symmetric)")
	}
	if !state.Associations["platform"]["security"] {
		t.Error("platform->security should be associated (symmetric)")
	}
}

func TestLoadTopologyState_NoAssignments(t *testing.T) {
	db := testutil.SetupDB(t)

	name := "noassign"
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          name,
				Cidr:          "10.70.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          name + "-i",
				Cidr:          "10.71.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: time.Now(),
		},
		&service.Cidr{
			Name:   "noassign",
			Cidr:   "10.70.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server",
			Cidr:     "10.70.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server",
			Route:     "10.70.0.1/32",
			PublicKey: "pub-server",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}

	if _, err := db.InsertGroup("noassign", "lonely"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	state, err := db.LoadTopologyState("noassign")
	if err != nil {
		t.Fatalf("load topology state: %v", err)
	}

	if len(state.Cidrs) != 2 {
		t.Fatalf("expected 2 cidrs, got %d", len(state.Cidrs))
	}
	if len(state.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(state.Peers))
	}
	if len(state.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(state.Assignments))
	}
	if len(state.Associations) != 0 {
		t.Errorf("expected 0 associations, got %d", len(state.Associations))
	}
}

func TestLoadTopologyState_NonexistentNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	if _, err := db.LoadTopologyState("doesnotexist"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("load topology state error = %v, want ErrNotFound", err)
	}
}

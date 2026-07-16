package database_test

import (
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
			Name:     "cord-server-cidr",
			Cidr:     "10.50.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server-cidr",
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
			Name:     "alice-host",
			Cidr:     "10.50.1.5/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		{
			Name:     "bob-host",
			Cidr:     "10.50.2.5/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
	} {
		if err := db.InsertCidr("toponet", &c); err != nil {
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
		{"alice-host", "platform"},
	} {
		if err := db.AssignGroup("toponet", pair[0], pair[1]); err != nil {
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
			CidrName:  "alice-host",
			Route:     "10.50.1.5/32",
			PublicKey: "pub-alice",
			Admin:     false,
			Enabled:   true,
			Confirmed: true,
		},
		{
			Name:      "bob",
			CidrName:  "bob-host",
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

func TestLoadTopologySnapshot_Cidrs(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	snap, err := db.LoadTopologySnapshot("toponet")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Cidrs) != 6 {
		t.Fatalf("expected 6 cidrs, got %d", len(snap.Cidrs))
	}

	byName := make(map[string]bool)
	for _, c := range snap.Cidrs {
		byName[c.Name] = true
	}
	for _, want := range []string{"toponet", "cord-server-cidr", "subnet-a", "subnet-b", "alice-host", "bob-host"} {
		if !byName[want] {
			t.Errorf("missing cidr %q", want)
		}
	}

	for _, c := range snap.Cidrs {
		if c.Name == "cord-server-cidr" {
			if !c.Terminal {
				t.Error("cord-server-cidr should be terminal")
			}
			if c.Cidr != "10.50.0.1/32" {
				t.Errorf("cord-server-cidr cidr = %q, want 10.50.0.1/32", c.Cidr)
			}
			if c.Prefix != 32 {
				t.Errorf("cord-server-cidr prefix = %d, want 32", c.Prefix)
			}
			if c.Bits != 32 {
				t.Errorf("cord-server-cidr bits = %d, want 32", c.Bits)
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

func TestLoadTopologySnapshot_Assignments(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	snap, err := db.LoadTopologySnapshot("toponet")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Assignments) == 0 {
		t.Fatal("expected assignments, got none")
	}

	groups := snap.Assignments["subnet-a"]
	if len(groups) != 1 || groups[0] != "devops" {
		t.Errorf("subnet-a assignments = %v, want [devops]", groups)
	}

	groups = snap.Assignments["alice-host"]
	if len(groups) != 1 || groups[0] != "platform" {
		t.Errorf("alice-host assignments = %v, want [platform]", groups)
	}

	if _, ok := snap.Assignments["subnet-b"]; ok {
		t.Error("subnet-b should have no assignments")
	}
}

func TestLoadTopologySnapshot_Associations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	snap, err := db.LoadTopologySnapshot("toponet")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Associations) != 2 {
		t.Fatalf("expected 2 groups in associations, got %d", len(snap.Associations))
	}

	if !snap.Associations["devops"]["platform"] {
		t.Error("devops->platform should be associated")
	}
	if !snap.Associations["platform"]["devops"] {
		t.Error("platform->devops should be associated (symmetric)")
	}
}

func TestLoadTopologySnapshot_Peers(t *testing.T) {
	db := testutil.SetupDB(t)
	seedTopologyNetwork(t, db)

	snap, err := db.LoadTopologySnapshot("toponet")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.PeerCidr) != 3 {
		t.Fatalf("expected 3 peer cidr entries, got %d", len(snap.PeerCidr))
	}
	if len(snap.PeerInfo) != 3 {
		t.Fatalf("expected 3 peer info entries, got %d", len(snap.PeerInfo))
	}

	if snap.PeerCidr["alice"] != "alice-host" {
		t.Errorf("alice cidr = %q, want alice-host", snap.PeerCidr["alice"])
	}
	if snap.PeerCidr["bob"] != "bob-host" {
		t.Errorf("bob cidr = %q, want bob-host", snap.PeerCidr["bob"])
	}
	if snap.PeerCidr["cord-server"] != "cord-server-cidr" {
		t.Errorf("cord-server cidr = %q, want cord-server-cidr", snap.PeerCidr["cord-server"])
	}

	info := snap.PeerInfo["alice"]
	if info.Name != "alice" {
		t.Errorf("name = %q, want alice", info.Name)
	}
	if info.PublicKey != "pub-alice" {
		t.Errorf("public_key = %q, want pub-alice", info.PublicKey)
	}
	if info.Route != "10.50.1.5/32" {
		t.Errorf("route = %q, want 10.50.1.5/32", info.Route)
	}
}

func TestLoadTopologySnapshot_EmptyNetwork(t *testing.T) {
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
			Name:     "cord-server-cidr",
			Cidr:     "10.60.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server-cidr",
			Route:     "10.60.0.1/32",
			PublicKey: "pub-server",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}

	snap, err := db.LoadTopologySnapshot("emptynet")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Cidrs) != 2 {
		t.Fatalf("expected 2 cidrs (root + server), got %d", len(snap.Cidrs))
	}

	if len(snap.PeerCidr) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(snap.PeerCidr))
	}
	if _, ok := snap.PeerCidr["cord-server"]; !ok {
		t.Error("missing cord-server peer")
	}

	if len(snap.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(snap.Assignments))
	}
	if len(snap.Associations) != 0 {
		t.Errorf("expected 0 associations, got %d", len(snap.Associations))
	}
}

func TestLoadTopologySnapshot_MultipleAssociations(t *testing.T) {
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

	snap, err := db.LoadTopologySnapshot("toponet")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Associations) != 3 {
		t.Fatalf("expected 3 groups in associations, got %d", len(snap.Associations))
	}

	if !snap.Associations["security"]["devops"] {
		t.Error("security->devops should be associated")
	}
	if !snap.Associations["security"]["platform"] {
		t.Error("security->platform should be associated")
	}
	if !snap.Associations["devops"]["security"] {
		t.Error("devops->security should be associated (symmetric)")
	}
	if !snap.Associations["platform"]["security"] {
		t.Error("platform->security should be associated (symmetric)")
	}
}

func TestLoadTopologySnapshot_NoAssignments(t *testing.T) {
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
			Name:     "cord-server-cidr",
			Cidr:     "10.70.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server-cidr",
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

	snap, err := db.LoadTopologySnapshot("noassign")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Cidrs) != 2 {
		t.Fatalf("expected 2 cidrs, got %d", len(snap.Cidrs))
	}
	if len(snap.PeerCidr) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(snap.PeerCidr))
	}
	if len(snap.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(snap.Assignments))
	}
	if len(snap.Associations) != 0 {
		t.Errorf("expected 0 associations, got %d", len(snap.Associations))
	}
}

func TestLoadTopologySnapshot_NonexistentNetwork(t *testing.T) {
	db := testutil.SetupDB(t)

	snap, err := db.LoadTopologySnapshot("doesnotexist")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snap.Cidrs) != 0 {
		t.Errorf("expected 0 cidrs, got %d", len(snap.Cidrs))
	}
	if len(snap.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(snap.Assignments))
	}
	if len(snap.Associations) != 0 {
		t.Errorf("expected 0 associations, got %d", len(snap.Associations))
	}
	if len(snap.PeerCidr) != 0 {
		t.Errorf("expected 0 peer cidr, got %d", len(snap.PeerCidr))
	}
	if len(snap.PeerInfo) != 0 {
		t.Errorf("expected 0 peer info, got %d", len(snap.PeerInfo))
	}
}

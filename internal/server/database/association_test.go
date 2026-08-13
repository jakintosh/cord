package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForAssoc(t *testing.T, db *database.DB) {
	t.Helper()

	name := "assocnet"
	if err := db.BootstrapNetwork(
		&service.NetworkConfig{
			Name:       name,
			PrivateKey: "priv-" + name,
			PublicKey:  "pub-" + name,
			ExternalIP: "1.1.1.1",
			Main: service.PlaneConfig{
				Name:          name,
				Cidr:          "10.0.0.0/16",
				WireguardPort: 51820,
				ApiPort:       8080,
			},
			Invite: service.PlaneConfig{
				Name:          name + "-i",
				Cidr:          "10.1.0.0/24",
				WireguardPort: 51821,
				ApiPort:       8080,
			},
			CreatedAt: time.Now(),
		},
		&service.Cidr{
			Name:   "assocnet",
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server-cidr",
			Cidr:     "10.0.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			CidrName:  "cord-server-cidr",
			PublicKey: "pub",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}
	for _, c := range []service.Cidr{
		{Name: "subnet-a", Cidr: "10.0.1.0/24", Prefix: 24, Bits: 32},
		{Name: "subnet-b", Cidr: "10.0.2.0/24", Prefix: 24, Bits: 32},
		{Name: "subnet-c", Cidr: "10.0.3.0/24", Prefix: 24, Bits: 32},
	} {
		if err := db.CreateCidr("assocnet", &c); err != nil {
			t.Fatalf("seed cidr %s: %v", c.Name, err)
		}
	}
	for _, g := range []string{"group-a", "group-b", "group-c"} {
		if _, err := db.InsertGroup("assocnet", g); err != nil {
			t.Fatalf("seed group %s: %v", g, err)
		}
	}
}

func TestInsertAndListAssociation(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-a",
		Group2: "group-b",
	}); err != nil {
		t.Fatalf("insert association: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list associations: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association, got %d", len(assocs))
	}
	if assocs[0].Group1 != "group-a" || assocs[0].Group2 != "group-b" {
		t.Errorf("unexpected association: %+v", assocs[0])
	}
}

func TestInsertAssociation_ReversedOrderNormalized(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-b",
		Group2: "group-a",
	}); err != nil {
		t.Fatalf("insert association: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list associations: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association, got %d", len(assocs))
	}
	if assocs[0].Group1 != "group-a" || assocs[0].Group2 != "group-b" {
		t.Errorf("expected (group-a, group-b), got (%s, %s)", assocs[0].Group1, assocs[0].Group2)
	}
}

func TestInsertAssociation_Duplicate(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	a := &service.Association{Group1: "group-a", Group2: "group-b"}
	if err := db.InsertAssociation("assocnet", a); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-b",
		Group2: "group-a",
	})
	if err == nil {
		t.Fatal("expected error for duplicate association")
	}
}

func TestInsertAssociation_UnknownGroup(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-a",
		Group2: "ghost",
	})
	if err == nil {
		t.Fatal("expected error for unknown group")
	}
}

func TestDeleteAssociation(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-a",
		Group2: "group-b",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.DeleteAssociation("assocnet", "group-a", "group-b"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assocs) != 0 {
		t.Fatalf("expected 0 associations, got %d", len(assocs))
	}
}

func TestDeleteAssociation_ReversedOrder(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-a",
		Group2: "group-b",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.DeleteAssociation("assocnet", "group-b", "group-a"); err != nil {
		t.Fatalf("delete reversed: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assocs) != 0 {
		t.Fatalf("expected 0 associations, got %d", len(assocs))
	}
}

func TestListMultipleAssociations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	for _, pair := range [][2]string{
		{"group-a", "group-b"},
		{"group-a", "group-c"},
		{"group-b", "group-c"},
	} {
		if err := db.InsertAssociation("assocnet", &service.Association{
			Group1: pair[0],
			Group2: pair[1],
		}); err != nil {
			t.Fatalf("insert %v-%v: %v", pair[0], pair[1], err)
		}
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assocs) != 3 {
		t.Fatalf("expected 3 associations, got %d", len(assocs))
	}
}

func TestDeleteAssociation_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	err := db.DeleteAssociation("assocnet", "group-a", "group-b")
	if err != nil {
		t.Fatalf("delete nonexistent should not error: %v", err)
	}
}

func TestAssociation_CascadeOnGroupDelete(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-a",
		Group2: "group-b",
	}); err != nil {
		t.Fatalf("insert association: %v", err)
	}

	if err := db.DeleteGroup("assocnet", "group-a"); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(assocs) != 0 {
		t.Errorf("expected 0 associations after cascade, got %d", len(assocs))
	}
}

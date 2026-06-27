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
	now := time.Now()
	if err := db.BootstrapNetwork(&service.Network{
		Name:             "assocnet",
		PrivateKey:       "priv",
		PublicKey:        "pub",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.1.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        now,
	}, &service.Cidr{Name: "assocnet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("seed network: %v", err)
	}
	for _, c := range []service.Cidr{
		{Name: "subnet-a", Cidr: "10.0.1.0/24", Length: 24, Prefix: 32},
		{Name: "subnet-b", Cidr: "10.0.2.0/24", Length: 24, Prefix: 32},
		{Name: "subnet-c", Cidr: "10.0.3.0/24", Length: 24, Prefix: 32},
	} {
		c := c
		if err := db.InsertCidr("assocnet", &c); err != nil {
			t.Fatalf("seed cidr %s: %v", c.Name, err)
		}
	}
}

func TestInsertAndListAssociation(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Cidr1: "subnet-a",
		Cidr2: "subnet-b",
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
	if assocs[0].Cidr1 != "subnet-a" || assocs[0].Cidr2 != "subnet-b" {
		t.Errorf("unexpected association: %+v", assocs[0])
	}
}

func TestInsertAssociation_ReversedOrderNormalized(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Cidr1: "subnet-b",
		Cidr2: "subnet-a",
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
	if assocs[0].Cidr1 != "subnet-a" || assocs[0].Cidr2 != "subnet-b" {
		t.Errorf("expected (subnet-a, subnet-b), got (%s, %s)", assocs[0].Cidr1, assocs[0].Cidr2)
	}
}

func TestInsertAssociation_Duplicate(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	a := &service.Association{Cidr1: "subnet-a", Cidr2: "subnet-b"}
	if err := db.InsertAssociation("assocnet", a); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	err := db.InsertAssociation("assocnet", &service.Association{
		Cidr1: "subnet-b",
		Cidr2: "subnet-a",
	})
	if err == nil {
		t.Fatal("expected error for duplicate association")
	}
}

func TestInsertAssociation_UnknownCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	err := db.InsertAssociation("assocnet", &service.Association{
		Cidr1: "subnet-a",
		Cidr2: "ghost",
	})
	if err == nil {
		t.Fatal("expected error for unknown cidr")
	}
}

func TestDeleteAssociation(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Cidr1: "subnet-a",
		Cidr2: "subnet-b",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.DeleteAssociation("assocnet", "subnet-a", "subnet-b"); err != nil {
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
		Cidr1: "subnet-a",
		Cidr2: "subnet-b",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.DeleteAssociation("assocnet", "subnet-b", "subnet-a"); err != nil {
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
		{"subnet-a", "subnet-b"},
		{"subnet-a", "subnet-c"},
		{"subnet-b", "subnet-c"},
	} {
		if err := db.InsertAssociation("assocnet", &service.Association{
			Cidr1: pair[0],
			Cidr2: pair[1],
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

	err := db.DeleteAssociation("assocnet", "subnet-a", "subnet-b")
	if err != nil {
		t.Fatalf("delete nonexistent should not error: %v", err)
	}
}

func TestAssociation_CascadeOnCidrDelete(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Cidr1: "subnet-a",
		Cidr2: "subnet-b",
	}); err != nil {
		t.Fatalf("insert association: %v", err)
	}

	if err := db.DeleteCidr("assocnet", "subnet-a"); err != nil {
		t.Fatalf("delete cidr: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(assocs) != 0 {
		t.Errorf("expected 0 associations after cascade, got %d", len(assocs))
	}
}

package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForCidr(t *testing.T, db *database.DB) {
	t.Helper()
	now := time.Now()
	if err := db.BootstrapNetwork(&service.Network{
		Name:             "cidrnet",
		PrivateKey:       "priv",
		PublicKey:        "pub",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.1.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        now,
	}, &service.Cidr{Name: "cidrnet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

func TestInsertAndGetCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	cidr := &service.Cidr{
		Name:   "subnet-1",
		Cidr:   "10.0.64.0/24",
		Length: 24,
		Prefix: 32,
	}

	if err := db.InsertCidr("cidrnet", cidr); err != nil {
		t.Fatalf("insert cidr: %v", err)
	}

	got, err := db.GetCidr("cidrnet", "subnet-1")
	if err != nil {
		t.Fatalf("get cidr: %v", err)
	}

	if got.Name != cidr.Name {
		t.Errorf("name = %q, want %q", got.Name, cidr.Name)
	}
	if got.Cidr != cidr.Cidr {
		t.Errorf("cidr = %q, want %q", got.Cidr, cidr.Cidr)
	}
	if got.Length != cidr.Length {
		t.Errorf("length = %d, want %d", got.Length, cidr.Length)
	}
	if got.Prefix != cidr.Prefix {
		t.Errorf("prefix = %d, want %d", got.Prefix, cidr.Prefix)
	}
}

func TestGetCidr_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	_, err := db.GetCidr("cidrnet", "nope")
	if err == nil {
		t.Fatal("expected error for nonexistent cidr")
	}
}

func TestListCidrs(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.InsertCidr("cidrnet", &service.Cidr{
		Name: "ccc", Cidr: "10.0.1.0/24", Length: 24, Prefix: 32,
	}); err != nil {
		t.Fatalf("insert ccc: %v", err)
	}
	if err := db.InsertCidr("cidrnet", &service.Cidr{
		Name: "aaa", Cidr: "10.0.2.0/24", Length: 24, Prefix: 32,
	}); err != nil {
		t.Fatalf("insert aaa: %v", err)
	}

	cidrs, err := db.ListCidrs("cidrnet")
	if err != nil {
		t.Fatalf("list cidrs: %v", err)
	}
	if len(cidrs) != 3 {
		t.Fatalf("expected 3 cidrs, got %d", len(cidrs))
	}
	if cidrs[0].Name != "aaa" || cidrs[1].Name != "ccc" {
		t.Errorf("unexpected order: %v, %v", cidrs[0].Name, cidrs[1].Name)
	}
}

func TestInsertCidr_DuplicateName(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	cidr := &service.Cidr{Name: "dup", Cidr: "10.0.1.0/24", Length: 24, Prefix: 32}
	if err := db.InsertCidr("cidrnet", cidr); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	cidr.Cidr = "10.0.2.0/24"
	err := db.InsertCidr("cidrnet", cidr)
	if err == nil {
		t.Fatal("expected error for duplicate cidr name")
	}
}

func TestInsertCidr_DuplicateCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.InsertCidr("cidrnet", &service.Cidr{
		Name: "a", Cidr: "10.0.1.0/24", Length: 24, Prefix: 32,
	}); err != nil {
		t.Fatalf("insert a: %v", err)
	}

	err := db.InsertCidr("cidrnet", &service.Cidr{
		Name: "b", Cidr: "10.0.1.0/24", Length: 24, Prefix: 32,
	})
	if err == nil {
		t.Fatal("expected error for duplicate cidr range")
	}
}

func TestDeleteCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.InsertCidr("cidrnet", &service.Cidr{
		Name: "delme", Cidr: "10.0.5.0/24", Length: 24, Prefix: 32,
	}); err != nil {
		t.Fatalf("insert cidr: %v", err)
	}

	if err := db.DeleteCidr("cidrnet", "delme"); err != nil {
		t.Fatalf("delete cidr: %v", err)
	}

	_, err := db.GetCidr("cidrnet", "delme")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteCidr_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	err := db.DeleteCidr("cidrnet", "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent cidr")
	}
}

func TestUpdateCidr_Rename(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.InsertCidr("cidrnet", &service.Cidr{
		Name: "old-cidr", Cidr: "10.0.10.0/24", Length: 24, Prefix: 32,
	}); err != nil {
		t.Fatalf("insert cidr: %v", err)
	}

	got, err := db.UpdateCidr("cidrnet", "old-cidr", service.UpdateCidrRequest{
		Name: "new-cidr",
	})
	if err != nil {
		t.Fatalf("update cidr: %v", err)
	}
	if got.Name != "new-cidr" {
		t.Errorf("name = %q, want new-cidr", got.Name)
	}

	_, err = db.GetCidr("cidrnet", "new-cidr")
	if err != nil {
		t.Fatalf("should find renamd cidr: %v", err)
	}
}

func TestIPv6Cidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	cidr := &service.Cidr{
		Name:   "v6-subnet",
		Cidr:   "fd00:1::/64",
		Length: 64,
		Prefix: 128,
	}
	if err := db.InsertCidr("cidrnet", cidr); err != nil {
		t.Fatalf("insert ipv6 cidr: %v", err)
	}

	got, err := db.GetCidr("cidrnet", "v6-subnet")
	if err != nil {
		t.Fatalf("get ipv6 cidr: %v", err)
	}
	if got.Cidr != "fd00:1::/64" {
		t.Errorf("cidr = %q, want fd00:1::/64", got.Cidr)
	}
}

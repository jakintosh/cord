package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForCidr(t *testing.T, db *database.DB) {
	t.Helper()

	name := "cidrnet"
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
			Name:   name,
			Cidr:   "10.0.0.0/16",
			Prefix: 16,
			Bits:   32,
		},
		&service.Cidr{
			Name:     "cord-server",
			Cidr:     "10.0.0.1/32",
			Prefix:   32,
			Bits:     32,
			Terminal: true,
		},
		&service.Peer{
			Name:      "cord-server",
			Route:     "10.0.0.1/32",
			PublicKey: "pub",
			Admin:     true,
			Enabled:   true,
			Confirmed: true,
			CidrName:  "cord-server",
		},
	); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

func TestInsertAndGetCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	cidr := &service.Cidr{
		Name:   "subnet-1",
		Cidr:   "10.0.64.0/24",
		Prefix: 24,
		Bits:   32,
	}

	if err := db.CreateCidr("cidrnet", cidr); err != nil {
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
	if got.Prefix != cidr.Prefix {
		t.Errorf("prefix = %d, want %d", got.Prefix, cidr.Prefix)
	}
	if got.Bits != cidr.Bits {
		t.Errorf("bits = %d, want %d", got.Bits, cidr.Bits)
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

	if err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "ccc", Cidr: "10.0.1.0/24", Prefix: 24, Bits: 32,
	}); err != nil {
		t.Fatalf("insert ccc: %v", err)
	}
	if err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "aaa", Cidr: "10.0.2.0/24", Prefix: 24, Bits: 32,
	}); err != nil {
		t.Fatalf("insert aaa: %v", err)
	}

	cidrs, err := db.ListCidrs("cidrnet")
	if err != nil {
		t.Fatalf("list cidrs: %v", err)
	}
	if len(cidrs) != 4 {
		t.Fatalf("expected 4 cidrs, got %d", len(cidrs))
	}
	if cidrs[0].Name != "aaa" || cidrs[1].Name != "ccc" {
		t.Errorf("unexpected order: %v, %v", cidrs[0].Name, cidrs[1].Name)
	}
}

func TestCreateCidr_DuplicateName(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	cidr := &service.Cidr{Name: "dup", Cidr: "10.0.1.0/24", Prefix: 24, Bits: 32}
	if err := db.CreateCidr("cidrnet", cidr); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	cidr.Cidr = "10.0.2.0/24"
	err := db.CreateCidr("cidrnet", cidr)
	if err == nil {
		t.Fatal("expected error for duplicate cidr name")
	}
}

func TestCreateCidr_DuplicateCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "a", Cidr: "10.0.1.0/24", Prefix: 24, Bits: 32,
	}); err != nil {
		t.Fatalf("insert a: %v", err)
	}

	err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "b", Cidr: "10.0.1.0/24", Prefix: 24, Bits: 32,
	})
	if err == nil {
		t.Fatal("expected error for duplicate cidr range")
	}
}

func TestDeleteCidr(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "delme", Cidr: "10.0.5.0/24", Prefix: 24, Bits: 32,
	}); err != nil {
		t.Fatalf("insert cidr: %v", err)
	}
	if _, err := db.InsertGroup("cidrnet", "engineering"); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	if err := db.AssignCidrGroup("cidrnet", "delme", "engineering"); err != nil {
		t.Fatalf("assign group: %v", err)
	}

	if err := db.DeleteCidr("cidrnet", "delme"); err != nil {
		t.Fatalf("delete cidr: %v", err)
	}

	_, err := db.GetCidr("cidrnet", "delme")
	if err == nil {
		t.Fatal("expected error after delete")
	}
	var assignmentCount int
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM cidr_assignment`).Scan(&assignmentCount); err != nil {
		t.Fatalf("count assignments after delete: %v", err)
	}
	if assignmentCount != 0 {
		t.Fatalf("assignments after CIDR delete = %d, want 0", assignmentCount)
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

func TestDeleteCidr_RejectsRoot(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	err := db.DeleteCidr("cidrnet", "cidrnet")
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("delete root CIDR: err = %v, want ErrConflict", err)
	}
	if _, err := db.GetCidr("cidrnet", "cidrnet"); err != nil {
		t.Fatalf("get root CIDR after rejected deletion: %v", err)
	}
}

func TestUpdateCidr_Rename(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "old-cidr", Cidr: "10.0.10.0/24", Prefix: 24, Bits: 32,
	}); err != nil {
		t.Fatalf("insert cidr: %v", err)
	}

	got, err := db.UpdateCidr("cidrnet", "old-cidr", "new-cidr")
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

func TestCreateCidr_RejectsOutsideMainCIDR(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	err := db.CreateCidr("cidrnet", &service.Cidr{
		Name:   "v6-subnet",
		Cidr:   "fd00:1::/64",
		Prefix: 64,
		Bits:   128,
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("create outside main CIDR: err = %v, want ErrInvalidInput", err)
	}
	if _, err := db.GetCidr("cidrnet", "v6-subnet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("get rejected CIDR: err = %v, want ErrNotFound", err)
	}
}

func TestCidrGroups_AssignListRemove(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if err := db.CreateCidr("cidrnet", &service.Cidr{
		Name: "servers", Cidr: "10.0.20.0/24", Prefix: 24, Bits: 32,
	}); err != nil {
		t.Fatalf("insert CIDR: %v", err)
	}
	for _, name := range []string{"engineering", "operations"} {
		if _, err := db.InsertGroup("cidrnet", name); err != nil {
			t.Fatalf("insert group %q: %v", name, err)
		}
	}
	if err := db.AssignCidrGroup("cidrnet", "servers", "operations"); err != nil {
		t.Fatalf("assign operations: %v", err)
	}
	if err := db.AssignCidrGroup("cidrnet", "servers", "engineering"); err != nil {
		t.Fatalf("assign engineering: %v", err)
	}
	if err := db.AssignCidrGroup("cidrnet", "servers", "engineering"); err == nil {
		t.Fatal("expected duplicate assignment to fail")
	}

	groups, err := db.ListCidrGroups("cidrnet", "servers")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 2 || groups[0].Name != "engineering" || groups[1].Name != "operations" {
		t.Fatalf("groups = %+v, want engineering, operations", groups)
	}

	if err := db.RemoveCidrGroup("cidrnet", "servers", "engineering"); err != nil {
		t.Fatalf("remove engineering: %v", err)
	}
	groups, err = db.ListCidrGroups("cidrnet", "servers")
	if err != nil {
		t.Fatalf("list groups after removal: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "operations" {
		t.Fatalf("groups after removal = %+v, want operations", groups)
	}
}

func TestCidrGroups_RejectsProvisionalPeer(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)
	testutil.SeedPeerDB(
		t,
		db,
		"cidrnet",
		"pending",
		"10.0.30.5/32",
		"pending-key",
		false,
		true,
		false,
	)
	if _, err := db.InsertGroup("cidrnet", "engineering"); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	if err := db.AssignCidrGroup("cidrnet", "pending", "engineering"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("assign provisional peer group: err = %v, want ErrConflict", err)
	}
}

func TestCidrGroups_MissingResources(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForCidr(t, db)

	if _, err := db.ListCidrGroups("cidrnet", "missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("list groups for missing CIDR: err = %v, want ErrNotFound", err)
	}
	if _, err := db.InsertGroup("cidrnet", "engineering"); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	if err := db.RemoveCidrGroup("cidrnet", "missing", "engineering"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("remove from missing CIDR: err = %v, want ErrNotFound", err)
	}
	if err := db.RemoveCidrGroup("cidrnet", "cidrnet", "missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("remove missing group: err = %v, want ErrNotFound", err)
	}
	if err := db.RemoveCidrGroup("cidrnet", "cidrnet", "engineering"); err != nil {
		t.Fatalf("remove absent assignment: %v", err)
	}
}

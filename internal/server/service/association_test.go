package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedTwoCIDRs(
	t *testing.T,
	svc *service.Service,
) {
	t.Helper()
	if err := svc.CreateCidr("testnet", "cidr-a", "10.0.1.0/24"); err != nil {
		t.Fatalf("seed cidr-a: %v", err)
	}
	if err := svc.CreateCidr("testnet", "cidr-b", "10.0.2.0/24"); err != nil {
		t.Fatalf("seed cidr-b: %v", err)
	}
}

func TestAddAssociation_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedTwoCIDRs(t, env.Service)

	err := env.Service.CreateAssociation("testnet", "cidr-a", "cidr-b")
	if err != nil {
		t.Fatalf("add association: %v", err)
	}

	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association, got %d", len(assocs))
	}
	if assocs[0].Cidr1 != "cidr-a" || assocs[0].Cidr2 != "cidr-b" {
		t.Errorf("unexpected: %s <-> %s", assocs[0].Cidr1, assocs[0].Cidr2)
	}
}

func TestAddAssociation_SameCIDR(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "solo", "10.0.1.0/24"); err != nil {
		t.Fatalf("add cidr: %v", err)
	}

	err := env.Service.CreateAssociation("testnet", "solo", "solo")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddAssociation_EmptyCIDR(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.CreateAssociation("testnet", "", "cidr-b")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("empty cidr1: err = %v, want ErrInvalidInput", err)
	}

	err = env.Service.CreateAssociation("testnet", "cidr-a", "")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("empty cidr2: err = %v, want ErrInvalidInput", err)
	}
}

func TestAddAssociation_UnknownCIDR(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "known", "10.0.1.0/24"); err != nil {
		t.Fatalf("add cidr: %v", err)
	}

	err := env.Service.CreateAssociation("testnet", "known", "unknown")
	if !errors.Is(err, service.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestListAssociations_Empty(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assocs) != 0 {
		t.Fatalf("expected 0 associations, got %d", len(assocs))
	}
}

func TestRemoveAssociation_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedTwoCIDRs(t, env.Service)

	if err := env.Service.CreateAssociation("testnet", "cidr-a", "cidr-b"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := env.Service.DeleteAssociation("testnet", "cidr-a", "cidr-b"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(assocs) != 0 {
		t.Errorf("expected 0 associations, got %d", len(assocs))
	}
}

func TestRemoveAssociation_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	// DeleteAssociation does not error when the association doesn't exist
	// (0 rows affected is not an error for deletes in the current store).
	err := env.Service.DeleteAssociation("testnet", "a", "b")
	if err != nil {
		t.Fatalf("remove nonexistent: %v", err)
	}
}

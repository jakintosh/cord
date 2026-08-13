package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedTwoGroups(
	t *testing.T,
	svc *service.Service,
) {
	t.Helper()
	if _, err := svc.CreateGroup("testnet", "group-a"); err != nil {
		t.Fatalf("seed group-a: %v", err)
	}
	if _, err := svc.CreateGroup("testnet", "group-b"); err != nil {
		t.Fatalf("seed group-b: %v", err)
	}
}

func TestAddAssociation_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedTwoGroups(t, env.Service)

	err := env.Service.CreateAssociation("testnet", "group-a", "group-b")
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
	if assocs[0].Group1 != "group-a" || assocs[0].Group2 != "group-b" {
		t.Errorf("unexpected: %s <-> %s", assocs[0].Group1, assocs[0].Group2)
	}
}

func TestAddAssociation_EmptyGroup(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.CreateAssociation("testnet", "", "group-b")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("empty group1: err = %v, want ErrInvalidInput", err)
	}

	err = env.Service.CreateAssociation("testnet", "group-a", "")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("empty group2: err = %v, want ErrInvalidInput", err)
	}
}

func TestAddAssociation_UnknownGroup(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := svcCreateGroup(env.Service, "known"); err != nil {
		t.Fatalf("add group: %v", err)
	}

	err := env.Service.CreateAssociation("testnet", "known", "unknown")
	if !errors.Is(err, service.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func svcCreateGroup(svc *service.Service, name string) (*service.Group, error) {
	return svc.CreateGroup("testnet", name)
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
	seedTwoGroups(t, env.Service)

	if err := env.Service.CreateAssociation("testnet", "group-a", "group-b"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := env.Service.DeleteAssociation("testnet", "group-a", "group-b"); err != nil {
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

	err := env.Service.DeleteAssociation("testnet", "a", "b")
	if err != nil {
		t.Fatalf("remove nonexistent: %v", err)
	}
}

package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func TestAddCidr_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "lan",
		Cidr: "10.0.1.0/24",
	})
	if err != nil {
		t.Fatalf("add cidr: %v", err)
	}

	cidr, err := env.svc.GetCidr("testnet", "lan")
	if err != nil {
		t.Fatalf("get cidr: %v", err)
	}

	if cidr.Name != "lan" {
		t.Errorf("name = %q, want lan", cidr.Name)
	}
	if cidr.Cidr != "10.0.1.0/24" {
		t.Errorf("cidr = %q, want 10.0.1.0/24", cidr.Cidr)
	}
	if cidr.Length != 24 {
		t.Errorf("length = %d, want 24", cidr.Length)
	}
	if cidr.Prefix != 32 {
		t.Errorf("prefix = %d, want 32", cidr.Prefix)
	}
}

func TestAddCidr_EmptyName(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "",
		Cidr: "10.0.1.0/24",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddCidr_InvalidFormat(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "bad",
		Cidr: "not-a-cidr",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddCidr_NotContainedInRoot(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "outside",
		Cidr: "192.168.1.0/24",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddCidr_Overlap(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	if err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "first",
		Cidr: "10.0.1.0/24",
	}); err != nil {
		t.Fatalf("first add: %v", err)
	}

	err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "second",
		Cidr: "10.0.1.0/24",
	})
	if !errors.Is(err, service.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestAddCidr_NonexistentNetwork(t *testing.T) {
	env := setupTestEnv(t)

	err := env.svc.AddCidr("nonexistent", service.CreateCidrRequest{
		Name: "lan",
		Cidr: "10.0.1.0/24",
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetCidr_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.GetCidr("testnet", "nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListCidrs_Empty(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	cidrs, err := env.svc.ListCidrs("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cidrs) != 1 {
		t.Fatalf("expected 1 cidr (root), got %d", len(cidrs))
	}
}

func TestListCidrs_Multiple(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	if err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "beta",
		Cidr: "10.0.2.0/24",
	}); err != nil {
		t.Fatalf("add beta: %v", err)
	}
	if err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "alpha",
		Cidr: "10.0.1.0/24",
	}); err != nil {
		t.Fatalf("add alpha: %v", err)
	}

	cidrs, err := env.svc.ListCidrs("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cidrs) != 3 {
		t.Fatalf("expected 3 cidrs, got %d", len(cidrs))
	}
	if cidrs[0].Name != "alpha" || cidrs[1].Name != "beta" || cidrs[2].Name != "testnet" {
		t.Errorf("unexpected order: %s, %s, %s", cidrs[0].Name, cidrs[1].Name, cidrs[2].Name)
	}
}

func TestRemoveCidr_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	if err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "removeme",
		Cidr: "10.0.1.0/24",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := env.svc.RemoveCidr("testnet", "removeme"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err := env.svc.GetCidr("testnet", "removeme")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after remove: err = %v, want ErrNotFound", err)
	}
}

func TestRemoveCidr_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.RemoveCidr("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateCidr_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	if err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "old-name",
		Cidr: "10.0.1.0/24",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := env.svc.UpdateCidr("testnet", "old-name", service.UpdateCidrRequest{
		Name: "new-name",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cidr, err := env.svc.GetCidr("testnet", "new-name")
	if err != nil {
		t.Fatalf("get renamed: %v", err)
	}
	if cidr.Name != "new-name" {
		t.Errorf("name = %q, want new-name", cidr.Name)
	}
}

func TestUpdateCidr_EmptyName(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	if err := env.svc.AddCidr("testnet", service.CreateCidrRequest{
		Name: "cidr1",
		Cidr: "10.0.1.0/24",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := env.svc.UpdateCidr("testnet", "cidr1", service.UpdateCidrRequest{
		Name: "",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

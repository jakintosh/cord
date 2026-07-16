package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAddCidr_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.CreateCidr("testnet", "lan", "10.0.1.0/24")
	if err != nil {
		t.Fatalf("add cidr: %v", err)
	}

	cidr, err := env.Service.GetCidr("testnet", "lan")
	if err != nil {
		t.Fatalf("get cidr: %v", err)
	}

	if cidr.Name != "lan" {
		t.Errorf("name = %q, want lan", cidr.Name)
	}
	if cidr.Cidr != "10.0.1.0/24" {
		t.Errorf("cidr = %q, want 10.0.1.0/24", cidr.Cidr)
	}
	if cidr.Prefix != 24 {
		t.Errorf("prefix = %d, want 24", cidr.Prefix)
	}
	if cidr.Bits != 32 {
		t.Errorf("bits = %d, want 32", cidr.Bits)
	}
}

func TestAddCidr_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.CreateCidr("testnet", "", "10.0.1.0/24")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddCidr_InvalidFormat(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.CreateCidr("testnet", "bad", "not-a-cidr")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddCidr_NotContainedInRoot(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.CreateCidr("testnet", "outside", "192.168.1.0/24")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddCidr_Overlap(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "first", "10.0.1.0/24"); err != nil {
		t.Fatalf("first add: %v", err)
	}

	err := env.Service.CreateCidr("testnet", "second", "10.0.1.0/24")
	if !errors.Is(err, service.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestAddCidr_NonexistentNetwork(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.CreateCidr("nonexistent", "lan", "10.0.1.0/24")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetCidr_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.GetCidr("testnet", "nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListCidrs_Empty(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	cidrs, err := env.Service.ListCidrs("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cidrs) != 2 {
		t.Fatalf("expected 2 cidrs (root + server), got %d", len(cidrs))
	}
}

func TestListCidrs_Multiple(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "beta", "10.0.2.0/24"); err != nil {
		t.Fatalf("add beta: %v", err)
	}
	if err := env.Service.CreateCidr("testnet", "alpha", "10.0.1.0/24"); err != nil {
		t.Fatalf("add alpha: %v", err)
	}

	cidrs, err := env.Service.ListCidrs("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cidrs) != 4 {
		t.Fatalf("expected 4 cidrs, got %d", len(cidrs))
	}
	if cidrs[0].Name != "alpha" || cidrs[1].Name != "beta" || cidrs[2].Name != "cord-server" || cidrs[3].Name != "testnet" {
		t.Errorf("unexpected order: %s, %s, %s, %s", cidrs[0].Name, cidrs[1].Name, cidrs[2].Name, cidrs[3].Name)
	}
}

func TestRemoveCidr_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "removeme", "10.0.1.0/24"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if err := env.Service.DeleteCidr("testnet", "removeme"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err := env.Service.GetCidr("testnet", "removeme")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after remove: err = %v, want ErrNotFound", err)
	}
}

func TestRemoveCidr_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.DeleteCidr("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateCidr_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "old-name", "10.0.1.0/24"); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := env.Service.UpdateCidr("testnet", "old-name", "new-name")
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cidr, err := env.Service.GetCidr("testnet", "new-name")
	if err != nil {
		t.Fatalf("get renamed: %v", err)
	}
	if cidr.Name != "new-name" {
		t.Errorf("name = %q, want new-name", cidr.Name)
	}
}

func TestUpdateCidr_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "cidr1", "10.0.1.0/24"); err != nil {
		t.Fatalf("add: %v", err)
	}

	err := env.Service.UpdateCidr("testnet", "cidr1", "")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

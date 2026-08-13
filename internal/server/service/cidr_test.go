package service_test

import (
	"errors"
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

type createCidrStoreSpy struct {
	service.Store
	getNetworkCalls int
	createdNetwork  string
	createdCidr     *service.Cidr
}

func (s *createCidrStoreSpy) GetNetwork(string) (*service.NetworkConfig, error) {
	s.getNetworkCalls++
	return nil, errors.New("unexpected GetNetwork call")
}

func (s *createCidrStoreSpy) CreateCidr(network string, cidr *service.Cidr) error {
	s.createdNetwork = network
	s.createdCidr = cidr
	return nil
}

func TestCreateCidr_DelegatesPersistedContainmentToStore(t *testing.T) {
	store := &createCidrStoreSpy{}
	mgr := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())
	svc, err := service.New(service.Options{Store: store, WireGuard: mgr})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.CreateCidr("testnet", "lan", "10.0.1.0/24"); err != nil {
		t.Fatalf("create CIDR: %v", err)
	}
	if store.getNetworkCalls != 0 {
		t.Fatalf("GetNetwork calls = %d, want 0", store.getNetworkCalls)
	}
	if store.createdNetwork != "testnet" {
		t.Fatalf("created network = %q, want testnet", store.createdNetwork)
	}
	if store.createdCidr == nil || store.createdCidr.Cidr != "10.0.1.0/24" {
		t.Fatalf("created CIDR = %+v, want 10.0.1.0/24", store.createdCidr)
	}
}

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

func TestAddCidr_HostBitsSet(t *testing.T) {
	store := &createCidrStoreSpy{}
	mgr := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())
	svc, err := service.New(service.Options{Store: store, WireGuard: mgr})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.CreateCidr("testnet", "bad", "10.0.99.0/16")
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if got := err.Error(); got != `invalid input: invalid CIDR "10.0.99.0/16": host bits are set; network address is "10.0.0.0/16"` {
		t.Fatalf("err = %q", got)
	}
	if store.createdCidr != nil {
		t.Fatalf("CreateCidr called after rejected input: %+v", store.createdCidr)
	}
}

func TestAddCidr_CanonicalizesBeforePersistence(t *testing.T) {
	store := &createCidrStoreSpy{}
	mgr := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())
	svc, err := service.New(service.Options{Store: store, WireGuard: mgr})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.CreateCidr("testnet", "v6", "FD00:0:0:1::/64"); err != nil {
		t.Fatalf("create CIDR: %v", err)
	}
	if store.createdCidr == nil || store.createdCidr.Cidr != "fd00:0:0:1::/64" {
		t.Fatalf("created CIDR = %+v, want canonical fd00:0:0:1::/64", store.createdCidr)
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

func TestAddCidr_RegistrationReservationConflict(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.50")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if err := env.Service.CreateCidr("testnet", "reserved", "10.0.0.50/32"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("CIDR steals registration route: err = %v, want ErrConflict", err)
	}
	if err := env.Service.CreateCidr("testnet", "alice", "10.0.0.51/32"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("CIDR steals registration name: err = %v, want ErrConflict", err)
	}
	if err := env.Service.RevokeRegistration("testnet", "alice"); err != nil {
		t.Fatalf("revoke registration: %v", err)
	}
	if err := env.Service.CreateCidr("testnet", "reserved", "10.0.0.50/32"); err != nil {
		t.Fatalf("reuse released route for CIDR: %v", err)
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

func TestCidrGroups_AddListRemove(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.CreateCidr("testnet", "servers", "10.0.1.0/24"); err != nil {
		t.Fatalf("create CIDR: %v", err)
	}
	for _, name := range []string{"engineering", "operations"} {
		if _, err := env.Service.CreateGroup("testnet", name); err != nil {
			t.Fatalf("create group %q: %v", name, err)
		}
	}
	if err := env.Service.AssignCidrGroup("testnet", "servers", "operations"); err != nil {
		t.Fatalf("assign operations: %v", err)
	}
	if err := env.Service.AssignCidrGroup("testnet", "servers", "engineering"); err != nil {
		t.Fatalf("assign engineering: %v", err)
	}

	groups, err := env.Service.ListCidrGroups("testnet", "servers")
	if err != nil {
		t.Fatalf("list CIDR groups: %v", err)
	}
	if len(groups) != 2 || groups[0].Name != "engineering" || groups[1].Name != "operations" {
		t.Fatalf("CIDR groups = %+v, want engineering, operations", groups)
	}

	if err := env.Service.RemoveCidrGroup("testnet", "servers", "engineering"); err != nil {
		t.Fatalf("remove engineering: %v", err)
	}
	groups, err = env.Service.ListCidrGroups("testnet", "servers")
	if err != nil {
		t.Fatalf("list CIDR groups after removal: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "operations" {
		t.Fatalf("CIDR groups after removal = %+v, want operations", groups)
	}
}

func TestCidrGroups_MissingCidr(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.ListCidrGroups("testnet", "missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("list missing CIDR groups: err = %v, want ErrNotFound", err)
	}
	if err := env.Service.RemoveCidrGroup("testnet", "missing", "engineering"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("remove group from missing CIDR: err = %v, want ErrNotFound", err)
	}
}

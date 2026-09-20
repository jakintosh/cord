package testutil_test

import (
	"errors"
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestSetupDB_IsolatesDatabaseState(
	t *testing.T,
) {
	first := testutil.SetupDB(t)
	second := testutil.SetupDB(t)
	firstService, err := service.New(service.Options{Store: first})
	if err != nil {
		t.Fatalf("new first service: %v", err)
	}
	secondService, err := service.New(service.Options{Store: second})
	if err != nil {
		t.Fatalf("new second service: %v", err)
	}

	testutil.SeedNetwork(t, firstService)
	before, err := first.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get first network: %v", err)
	}
	if _, err := second.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("second database before seeding: got %v, want ErrNotFound", err)
	}

	// The same name can be seeded and changed independently in another database.
	testutil.SeedNetwork(t, secondService)
	if err := second.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable second network: %v", err)
	}
	changed, err := second.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get second network: %v", err)
	}
	if !changed.Enabled {
		t.Fatal("second network should be enabled")
	}
	unchanged, err := first.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get first network after second mutation: %v", err)
	}
	if !reflect.DeepEqual(unchanged, before) {
		t.Fatalf("first network changed: got %+v, want %+v", unchanged, before)
	}
}

package testutil_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestSetupDB_IsolatesDatabaseState(
	t *testing.T,
) {
	first := testutil.SetupDB(t)
	second := testutil.SetupDB(t)
	testutil.SeedNetworkDirect(t, first, "homenet")

	if _, err := second.GetNetwork("homenet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("second database get network: %v, want ErrNotFound", err)
	}

	testutil.SeedNetworkDirect(t, second, "homenet")
	if err := second.SetNetworkEnabled("homenet", true); err != nil {
		t.Fatalf("enable second database network: %v", err)
	}

	network, err := first.GetNetwork("homenet")
	if err != nil {
		t.Fatalf("get first database network: %v", err)
	}
	if network.Enabled {
		t.Error("enabling the second database network changed the first database")
	}
}

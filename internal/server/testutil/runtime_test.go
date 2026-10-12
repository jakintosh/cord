package testutil_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestRunRuntime_JoinsBeforeDatabaseCleanup(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	rt, err := runtime.New(runtime.Options{
		Service:     env.Service,
		WireGuard:   wireguard.NewManagerWithBackend(backend),
		DNSDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// This probe runs after RunRuntime's cleanup and before SetupDB's cleanup.
	t.Cleanup(func() {
		if err := env.Database.Conn.Ping(); err != nil {
			t.Fatalf("database closed before runtime cleanup: %v", err)
		}
		for _, name := range []string{"testnet", "testnet-i"} {
			if err := backend.Device(name).Check(); err == nil {
				t.Errorf("device %s still running after runtime cleanup", name)
			}
		}
	})
	testutil.RunRuntime(t, rt, runtime.Handlers{})
}

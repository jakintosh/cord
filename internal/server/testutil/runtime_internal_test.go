package testutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

// The subprocess observes process failure and cleanup ordering. Private access
// only shortens the real startup/shutdown deadline; a public timeout option
// would exist solely to test this mechanical resource-ownership guarantee.
func TestRunRuntime_CleanupContracts(
	t *testing.T,
) {
	const scenarioEnv = "CORD_SERVER_TESTUTIL_CLEANUP_CASE"
	if scenario := os.Getenv(scenarioEnv); scenario != "" {
		if scenario == "stuck startup" {
			t.Cleanup(func() { fmt.Println("DEPENDENCY_CLEANUP_REACHED") })
			db := SetupDB(t)
			svc, err := service.New(service.Options{Store: stalledStartupStore{Store: db}})
			if err != nil {
				t.Fatal(err)
			}
			rt, err := runtime.New(runtime.Options{
				Service:     svc,
				WireGuard:   wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend()),
				DNSDisabled: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			runRuntime(t, rt, runtime.Handlers{}, 25*time.Millisecond)
			t.Fatal("stalled startup unexpectedly returned")
		}

		env := SetupRuntime(t)
		SeedNetwork(t, env.Service)
		env.Enable(t, "testnet")
		device := env.Backend.Device("testnet")
		failure := errors.New("fixture cleanup sentinel")
		device.CloseErr = failure
		if scenario == "explicit error" {
			if err := env.Stop(); !errors.Is(err, failure) {
				t.Fatalf("stop = %v, want injected cleanup error", err)
			}
			// The fixture deliberately prevented release; repair its resource
			// after asserting the error, before automatic dependency cleanup.
			device.CloseErr = nil
			if err := device.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return
	}

	for _, tc := range []struct {
		name    string
		failure string
	}{
		{name: "explicit error"},
		{name: "implicit error", failure: "runtime cleanup:"},
		{name: "stuck startup", failure: "aborting test binary before dependency cleanup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunRuntime_CleanupContracts$", "-test.count=1")
			cmd.Env = append(os.Environ(), scenarioEnv+"="+tc.name)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("cleanup subprocess hung: %s", output)
			}
			if tc.failure == "" {
				if err != nil {
					t.Fatalf("explicitly asserted error failed cleanup: %v\n%s", err, output)
				}
				return
			}
			if err == nil || !strings.Contains(string(output), tc.failure) {
				t.Fatalf("subprocess error = %v, want %q\n%s", err, tc.failure, output)
			}
			if strings.Contains(string(output), "DEPENDENCY_CLEANUP_REACHED") {
				t.Fatalf("live runtime's dependencies were cleaned up: %s", output)
			}
		})
	}
}

// A store call is synchronous and has no cancellation parameter. This narrow
// fixture represents a startup dependency that never returns.
type stalledStartupStore struct {
	service.Store
}

func (s stalledStartupStore) ListNetworks() (
	[]*service.Network,
	error,
) {
	select {}
}

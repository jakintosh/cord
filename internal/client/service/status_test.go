package service_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestStatusReportsRefreshSchedules(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "mynet")

	status, err := env.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	disabled := status.Networks[0]
	if disabled.Sync.Cadence != 30*time.Second ||
		disabled.Scan.Cadence != 30*time.Second ||
		disabled.Report.Cadence != 30*time.Second {
		t.Fatalf("disabled refresh cadences = %+v, want 30s each", disabled)
	}
	if !disabled.Sync.LastRunAt.IsZero() {
		t.Fatalf("disabled last sync = %v, want zero", disabled.Sync.LastRunAt)
	}

	if err := env.Service.EnableNetwork("mynet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	// An attempted on-demand sync records completion and defers the next run,
	// even though this test intentionally has no peer API server.
	_ = env.Service.SyncNetwork("mynet")

	status, err = env.Service.Status()
	if err != nil {
		t.Fatalf("status after sync: %v", err)
	}
	running := status.Networks[0]
	if !running.Sync.LastRunAt.Equal(testutil.FixedTime) {
		t.Fatalf("last sync = %v, want %v", running.Sync.LastRunAt, testutil.FixedTime)
	}
	if !running.Scan.LastRunAt.IsZero() || !running.Report.LastRunAt.IsZero() {
		t.Fatalf("scan/report should not have run yet: %+v", running)
	}
}

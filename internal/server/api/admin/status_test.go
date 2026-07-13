package admin_test

import (
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get status
	url := "/status"
	result := wire.TestGet[admin.Status](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "ok" {
		t.Fatalf("status = %q, want ok", data.Status)
	}
	if len(data.Networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data.Networks))
	}
}

func TestAPIStatus_IncludesNetworks(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// get status
	statusURL := "/status"
	result := wire.TestGet[admin.Status](env.Router, statusURL)

	// verify result
	data := result.ExpectOK(t)
	if len(data.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data.Networks))
	}
	net := data.Networks[0]
	if net.Name != "testnet" {
		t.Fatalf("name = %q, want testnet", net.Name)
	}
	if net.Enabled {
		t.Fatal("expected freshly created network to be disabled")
	}
	if net.Running {
		t.Fatal("expected freshly created network not to be running")
	}
	reconcile := net.Reconcile
	if reconcile.MaxIntervalSeconds != 300 {
		t.Fatalf("reconcile max interval = %d, want 300", reconcile.MaxIntervalSeconds)
	}
	if reconcile.LastRunAt != nil {
		t.Fatalf("disabled last reconcile = %v, want nil", reconcile.LastRunAt)
	}
}

func TestAPIStatus_IncludesRunningReconcileSchedule(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if err := env.Service.EnableNetwork("testnet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	result := wire.TestGet[admin.Status](env.Router, "/status")
	data := result.ExpectOK(t)
	reconcile := data.Networks[0].Reconcile
	wantLast := testutil.FixedTime.Format(time.RFC3339)
	if reconcile.LastRunAt == nil || *reconcile.LastRunAt != wantLast {
		t.Fatalf("last reconcile = %v, want %q", reconcile.LastRunAt, wantLast)
	}
}

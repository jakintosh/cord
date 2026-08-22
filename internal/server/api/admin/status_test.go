package admin_test

import (
	"errors"
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
	if data.Health != "healthy" {
		t.Fatalf("health = %q, want healthy", data.Health)
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
	if net.Health != "inactive" {
		t.Fatalf("health = %q, want inactive", net.Health)
	}
	reconcile := net.Reconcile
	if reconcile.IntervalSeconds != 300 {
		t.Fatalf("reconcile interval = %d, want 300", reconcile.IntervalSeconds)
	}
	if reconcile.LastAttemptAt != nil || reconcile.LastSuccessAt != nil {
		t.Fatalf("disabled reconcile = %+v, want no attempts", reconcile)
	}
}

func TestAPIStatus_IncludesRunningReconcileSchedule(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	wire.TestPost[admin.NetworkStatus](env.Router, "/networks/testnet/enable", "").ExpectOK(t)

	result := wire.TestGet[admin.Status](env.Router, "/status")
	data := result.ExpectOK(t)
	reconcile := data.Networks[0].Reconcile
	wantLast := testutil.FixedTime.Format(time.RFC3339)
	if reconcile.LastAttemptAt == nil || *reconcile.LastAttemptAt != wantLast {
		t.Fatalf("last reconcile attempt = %v, want %q", reconcile.LastAttemptAt, wantLast)
	}
	if reconcile.LastSuccessAt == nil || *reconcile.LastSuccessAt != wantLast {
		t.Fatalf("last reconcile success = %v, want %q", reconcile.LastSuccessAt, wantLast)
	}
}

func TestAPIStatus_ReportsDegradedRuntimeHealth(
	t *testing.T,
) {
	// setup running network with a reconciliation failure
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	wire.TestPost[admin.NetworkStatus](
		env.Router,
		"/networks/testnet/enable",
		"",
	).ExpectOK(t)
	env.Backend.Device("testnet").PeersErr = errors.New("read peers failed")
	if err := env.Runtime.Converge("testnet"); err != nil {
		t.Fatalf("converge: %v", err)
	}

	// get status
	result := wire.TestGet[admin.Status](env.Router, "/status")

	// verify aggregate and activity health
	data := result.ExpectOK(t)
	if data.Health != "degraded" {
		t.Fatalf("health = %q, want degraded", data.Health)
	}
	network := data.Networks[0]
	if network.Health != "degraded" {
		t.Fatalf("network health = %q, want degraded", network.Health)
	}
	if network.Reconcile.Error == "" {
		t.Fatal("reconcile error should describe the current failure")
	}
}

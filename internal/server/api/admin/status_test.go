package admin_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	api, err := admin.New(admin.Options{Runtime: env.Runtime, Logger: logging.Discard(), Version: "test-version"})
	if err != nil {
		t.Fatalf("new versioned API: %v", err)
	}

	// get status
	url := "/status"
	result := wire.TestGet[admin.Status](api.Router(), url)

	// verify result
	data := result.ExpectOK(t)
	if data.Version != "test-version" {
		t.Fatalf("version = %q, want test-version", data.Version)
	}
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

func TestAPIStatus_ExposesResourceTreeAndDeviceFailure(
	t *testing.T,
) {
	// Setup an owned network, then lose its device without reconciliation.
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	wire.TestPost[admin.NetworkStatus](env.Router, "/networks/testnet/enable", "").ExpectOK(t)
	if err := env.Backend.Device("testnet").Close(); err != nil {
		t.Fatal(err)
	}

	// Request the public snapshot.
	result := wire.TestGet[admin.Status](env.Router, "/status")
	result.ExpectStatus(t, http.StatusOK)
	status := result.ExpectOK(t)
	root := status.Snapshot
	if root.Kind != "runtime" || root.State != "running" || root.Target != "running" || root.Health != "degraded" {
		t.Fatalf("runtime snapshot = %+v", root)
	}
	if root.Activities["reconcile"].LastSuccessAt == nil || len(root.Children) != 1 {
		t.Fatalf("runtime activity or ownership missing: %+v", root)
	}
	network := root.Children[0]
	if network.Kind != "network" || network.Name != "testnet" || network.Health != "degraded" || len(network.Children) != 2 {
		t.Fatalf("network snapshot = %+v", network)
	}
	if network.Activities["reconcile"].LastSuccessAt == nil {
		t.Fatal("network reconciliation activity missing")
	}
	main, invite := network.Children[0], network.Children[1]
	if main.Kind != "plane" || main.Name != "main" || invite.Name != "invite" {
		t.Fatalf("plane identities = %+v", network.Children)
	}
	// This environment configures neither DNS nor HTTP; neither should appear.
	if len(main.Children) != 1 || len(invite.Children) != 1 {
		t.Fatalf("unexpected unconfigured resources: %+v", network.Children)
	}
	device := main.Children[0]
	if device.Kind != "wireguard-device" || device.State != "failed" || device.Target != "running" || device.Error == "" {
		t.Fatalf("device snapshot = %+v", device)
	}
	if invite.Health != "healthy" || invite.Children[0].State != "running" {
		t.Fatalf("device failure affected sibling: %+v", invite)
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
	if len(data.Snapshot.Children) != 0 {
		t.Fatal("expected freshly created network not to be running")
	}
	if net.Health != "inactive" {
		t.Fatalf("health = %q, want inactive", net.Health)
	}
	reconcile := net.Reconcile
	if reconcile.IntervalSeconds != 0 {
		t.Fatalf("disabled reconcile interval = %d, want 0", reconcile.IntervalSeconds)
	}
	if reconcile.LastAttemptAt != nil || reconcile.LastSuccessAt != nil {
		t.Fatalf("disabled reconcile = %+v, want no attempts", reconcile)
	}
}

func TestAPIStatus_IncludesRunningReconcileSchedule(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	enableURL := "/networks/testnet/enable"
	wire.TestPost[admin.NetworkStatus](env.Router, enableURL, "").ExpectOK(t)

	// request
	url := "/status"
	result := wire.TestGet[admin.Status](env.Router, url)

	// result
	data := result.ExpectOK(t)
	if len(data.Networks) != 1 || data.Networks[0].Name != "testnet" {
		t.Fatalf("networks = %+v, want testnet", data.Networks)
	}
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
	enableURL := "/networks/testnet/enable"
	wire.TestPost[admin.NetworkStatus](env.Router, enableURL, "").ExpectOK(t)
	env.Backend.Device("testnet").PeersErr = errors.New("read peers failed")
	// A successful intent update triggers reconciliation through the API.
	body := ""
	wire.TestPost[admin.NetworkStatus](env.Router, enableURL, body).ExpectOK(t)

	// get status
	url := "/status"
	result := wire.TestGet[admin.Status](env.Router, url)

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

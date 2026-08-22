package api_test

import (
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get status
	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "ok" {
		t.Fatalf("status = %q, want ok", data.Status)
	}
	if data.Health != "healthy" {
		t.Fatalf("health = %q, want healthy", data.Health)
	}
	if data.Networks == nil {
		t.Fatal("expected networks to be a non-nil empty list")
	}
	if len(data.Networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data.Networks))
	}
	if data.Installs == nil {
		t.Fatal("expected installs to be a non-nil empty list")
	}
	if len(data.Installs) != 0 {
		t.Fatalf("expected 0 installs, got %d", len(data.Installs))
	}
}

func TestAPIStatus_IncludesNetworks(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	data := result.ExpectOK(t)
	if len(data.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data.Networks))
	}
	network := data.Networks[0]
	if network.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", network.Name)
	}
	if network.Enabled {
		t.Fatal("seeded network should be disabled")
	}
	if network.Running {
		t.Fatal("seeded network should not be running")
	}
	if network.Health != "inactive" {
		t.Fatalf("health = %q, want inactive", network.Health)
	}
	if network.Sync.IntervalSeconds != 30 ||
		network.Scan.IntervalSeconds != 30 ||
		network.Report.IntervalSeconds != 30 {
		t.Fatalf("refresh intervals = sync %d, scan %d, report %d; want 30s each", network.Sync.IntervalSeconds, network.Scan.IntervalSeconds, network.Report.IntervalSeconds)
	}
	if network.Sync.LastAttemptAt != nil || network.Sync.LastSuccessAt != nil {
		t.Fatalf("disabled sync status = %+v, want no attempts", network.Sync)
	}
}

func TestAPIStatus_ReportsEnabledAndRunning(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "mynet")

	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	data := result.ExpectOK(t)
	if len(data.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data.Networks))
	}
	network := data.Networks[0]
	if !network.Enabled {
		t.Fatal("enabled network should report enabled")
	}
	if !network.Running {
		t.Fatal("enabled network should report running")
	}
	if network.Scan.LastAttemptAt != nil {
		t.Fatalf("last scan attempt = %v, want nil before first scan", *network.Scan.LastAttemptAt)
	}
}

func TestAPIStatus_IncludesInstalls(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedInstall(t, "installing")

	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	data := result.ExpectOK(t)
	if len(data.Networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data.Networks))
	}
	if len(data.Installs) != 1 {
		t.Fatalf("expected 1 install, got %d", len(data.Installs))
	}
	install := data.Installs[0]
	if install.Name != "installing" {
		t.Fatalf("name = %q, want installing", install.Name)
	}
	if install.State != "invited" {
		t.Fatalf("state = %q, want invited", install.State)
	}
}

package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPIStatus_ReportsConfiguredVersion(
	t *testing.T,
) {
	// setup versioned API over the shared runtime
	env := testutil.SetupRuntime(t)
	control, err := api.New(api.Options{Runtime: env.Runtime, Version: "v1.2.3"})
	if err != nil {
		t.Fatal(err)
	}

	// send request
	url := "/status"
	result := wire.TestGet[api.Status](control.Router(), url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if data.Version != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", data.Version)
	}
}

func TestAPIStatus_ResourceTree(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "office")

	// send request
	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	// verify response
	status := result.ExpectStatusOK(t, http.StatusOK)
	root := status.Snapshot
	if root.Kind != "runtime" || root.State != "running" || len(root.Children) != 1 {
		t.Fatalf("runtime resource = %+v", root)
	}
	network := root.Children[0]
	if network.Name != "office" || network.Activities["sync"].NextAttemptAt == nil {
		t.Fatalf("network status = %+v", network)
	}
	if len(network.Children) != 1 || len(network.Children[0].Children) != 1 {
		t.Fatalf("network resources = %+v", network)
	}
	tunnel := network.Children[0]
	device := tunnel.Children[0]
	if tunnel.Kind != "tunnel" || device.Kind != "wireguard-device" || device.Name != "office" {
		t.Fatalf("child resources = %+v", tunnel)
	}
}

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// send request
	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
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
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// send request
	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
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
	if len(data.Snapshot.Children) != 0 {
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
	// setup env
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "mynet")

	// send request
	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if len(data.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data.Networks))
	}
	network := data.Networks[0]
	if !network.Enabled {
		t.Fatal("enabled network should report enabled")
	}
	if len(data.Snapshot.Children) != 1 || data.Snapshot.Children[0].State != "running" {
		t.Fatal("enabled network should report running")
	}
	if network.Scan.LastAttemptAt != nil {
		t.Fatalf("last scan attempt = %v, want nil before first scan", *network.Scan.LastAttemptAt)
	}
}

func TestAPIStatus_IncludesInstalls(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedInstall(t, "installing")

	// send request
	url := "/status"
	result := wire.TestGet[api.Status](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
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

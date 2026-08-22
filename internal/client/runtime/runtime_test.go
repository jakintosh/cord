package runtime_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestStart_StartsEnabledNetworks(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	if err := env.Runtime.Start(t.Context()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	network := networkStatus(t, env.Runtime, "testnet")
	if !network.Enabled || !network.Running {
		t.Fatalf("status = %+v, want enabled and running", network)
	}
	if network.Reason != "" {
		t.Fatalf("reason = %q, want empty", network.Reason)
	}
	if env.Backend.Device("testnet") == nil {
		t.Fatal("expected the network device")
	}
}

func TestStart_LeavesDisabledNetworksStopped(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	if err := env.Runtime.Start(t.Context()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	network := networkStatus(t, env.Runtime, "testnet")
	if network.Enabled || network.Running {
		t.Fatalf("status = %+v, want disabled and stopped", network)
	}
	if network.Health != runtime.HealthInactive {
		t.Fatalf("health = %q, want inactive", network.Health)
	}
	if env.Backend.Device("testnet") != nil {
		t.Fatal("disabled network should not create a device")
	}
}

func TestConverge_FailedStartKeepsIntentAndReportsReason(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})

	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	if err := rt.Converge("testnet"); err == nil {
		t.Fatal("converge should fail while the device cannot be created")
	}

	stored, err := env.Service.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !stored.Enabled {
		t.Fatal("a failed start must not un-set the enabled flag")
	}

	network := networkStatus(t, rt, "testnet")
	if !network.Enabled || network.Running {
		t.Fatalf("status = %+v, want enabled and not running", network)
	}
	if network.Reason == "" {
		t.Fatal("status reason should explain the divergence")
	}
	if network.Health != runtime.HealthDegraded {
		t.Fatalf("health = %q, want degraded", network.Health)
	}
}

func TestRun_RetriesFailedStartOnTick(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, runtime.Options{Interval: 5 * time.Millisecond})

	if err := rt.Start(t.Context()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	network := networkStatus(t, rt, "testnet")
	if network.Running || network.Reason == "" {
		t.Fatalf("status = %+v, want stopped with a reason", network)
	}

	// Reset clears the injected error under the backend's own lock.
	backend.Reset()

	waitFor(t, func() bool {
		network := networkStatus(t, rt, "testnet")
		return network.Running && network.Reason == ""
	}, "network to start on a later tick")
}

func TestConverge_DisableStopsRunningNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	if !networkStatus(t, env.Runtime, "testnet").Running {
		t.Fatal("network should be running")
	}

	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatalf("disable network: %v", err)
	}
	env.Converge(t, "testnet")

	network := networkStatus(t, env.Runtime, "testnet")
	if network.Enabled || network.Running {
		t.Fatalf("status = %+v, want disabled and stopped", network)
	}
	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
}

// TestConverge_RestartsChangedConfiguration verifies that a stored
// change to a running network's tunnel takes effect without any
// disable/enable dance: the runtime restarts it under the new record.
func TestConverge_RestartsChangedConfiguration(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	original := env.Backend.Device("testnet")

	port := uint16(51820)
	if err := env.Service.UpdateNetwork(
		"testnet",
		service.NetworkOptions{ListenPort: &port},
	); err != nil {
		t.Fatalf("update network: %v", err)
	}
	env.Converge(t, "testnet")

	if !networkStatus(t, env.Runtime, "testnet").Running {
		t.Fatal("network should still be running after a restart")
	}
	if original.CloseCalls != 1 {
		t.Fatalf("original device close calls = %d, want 1", original.CloseCalls)
	}
	creates := env.Backend.CreateCalls
	if len(creates) != 2 {
		t.Fatalf("device creates = %d, want 2", len(creates))
	}
	if creates[1].ListenPort != port {
		t.Fatalf("restarted listen port = %d, want %d", creates[1].ListenPort, port)
	}
}

// TestConverge_StopsUninstalledNetwork verifies that a record deleted
// out from under a running network takes its device down.
func TestConverge_StopsUninstalledNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	if err := env.Service.UninstallNetwork("testnet"); err != nil {
		t.Fatalf("uninstall network: %v", err)
	}
	env.Converge(t, "testnet")

	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
	status, err := env.Runtime.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Networks) != 0 {
		t.Fatalf("status networks = %d, want 0", len(status.Networks))
	}
}

// TestUpdateNetwork_RestartsRunningNetwork verifies that updating a
// running network's configuration both returns the updated record and
// restarts the network under it.
func TestUpdateNetwork_RestartsRunningNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	original := env.Backend.Device("testnet")

	port := uint16(51820)
	network, err := env.Runtime.UpdateNetwork(
		"testnet",
		service.NetworkOptions{ListenPort: &port},
	)
	if err != nil {
		t.Fatalf("update network: %v", err)
	}
	if network.ListenPort != port {
		t.Fatalf("returned listen port = %d, want %d", network.ListenPort, port)
	}
	if original.CloseCalls != 1 {
		t.Fatalf("original device close calls = %d, want 1", original.CloseCalls)
	}
	creates := env.Backend.CreateCalls
	if len(creates) != 2 || creates[1].ListenPort != port {
		t.Fatalf("device creates = %+v, want a restart under port %d", creates, port)
	}
}

// TestSetNetworkEnabled_ReportsDivergence verifies that enabling a
// network that cannot start returns the recorded intent alongside why it
// is not running, rather than failing the operation.
func TestSetNetworkEnabled_ReportsDivergence(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, runtime.Options{})

	status, err := rt.SetNetworkEnabled("testnet", true)
	if err != nil {
		t.Fatalf("set network enabled: %v", err)
	}
	if !status.Enabled || status.Running {
		t.Fatalf("status = %+v, want enabled and not running", status)
	}
	if status.Reason == "" {
		t.Fatal("status reason should explain the divergence")
	}
}

func TestUninstallNetwork_StopsRunningNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	if err := env.Runtime.UninstallNetwork("testnet"); err != nil {
		t.Fatalf("uninstall network: %v", err)
	}

	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
}

func TestStop_StopsEverything(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	if err := env.Runtime.Start(t.Context()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	env.Runtime.Stop()

	network := networkStatus(t, env.Runtime, "testnet")
	if !network.Enabled || network.Running {
		t.Fatalf("status = %+v, want enabled intent with nothing running", network)
	}
	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
}

func TestSync_NotRunning(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	_, err := env.Runtime.Sync("testnet")
	if !errors.Is(err, service.ErrNetworkNotEnabled) {
		t.Errorf("err = %v, want ErrNetworkNotEnabled", err)
	}
}

// newRuntime builds a runtime over env with the mock WireGuard backend,
// filling in the dependencies every test shares.
func newRuntime(
	t *testing.T,
	env *testutil.ServiceEnv,
	backend *wireguardtest.MockBackend,
	opts runtime.Options,
) *runtime.Runtime {
	t.Helper()

	opts.Service = env.Service
	opts.WireGuard = wireguard.NewManagerWithBackend(backend)
	opts.Wake = env.Wake
	opts.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}
	opts.Logger = logging.Discard()
	if opts.Clock == nil {
		opts.Clock = func() time.Time { return testutil.FixedTime }
	}

	rt, err := runtime.New(opts)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(rt.Stop)
	return rt
}

// networkStatus returns the status of one network, failing the test when
// the runtime does not know it.
func networkStatus(
	t *testing.T,
	rt *runtime.Runtime,
	name string,
) runtime.NetworkStatus {
	t.Helper()

	status, err := rt.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, network := range status.Networks {
		if network.Name == name {
			return network
		}
	}
	t.Fatalf("network %q missing from status", name)
	return runtime.NetworkStatus{}
}

// waitFor polls until done reports true, or fails the test.
func waitFor(
	t *testing.T,
	done func() bool,
	what string,
) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

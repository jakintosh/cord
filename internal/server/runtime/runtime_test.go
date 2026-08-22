package runtime_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestStart_StartsEnabledNetworks(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
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
	if !network.Reconcile.LastRunAt.Equal(testutil.FixedTime) {
		t.Fatalf("last reconcile = %v, want %v", network.Reconcile.LastRunAt, testutil.FixedTime)
	}
	if env.Backend.Device("testnet") == nil || env.Backend.Device("testnet-i") == nil {
		t.Fatal("expected both plane devices")
	}
}

func TestStart_LeavesDisabledNetworksStopped(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Runtime.Start(t.Context()); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	network := networkStatus(t, env.Runtime, "testnet")
	if network.Enabled || network.Running {
		t.Fatalf("status = %+v, want disabled and stopped", network)
	}
	if env.Backend.Device("testnet") != nil {
		t.Fatal("disabled network should not create a device")
	}
}

func TestConverge_FailedStartKeepsIntentAndReportsReason(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, time.Hour)

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
}

func TestRun_RetriesFailedStartOnTick(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, 5*time.Millisecond)

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
	testutil.SeedNetwork(t, env.Service)
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
		t.Fatalf("main device close calls = %+v, want 1", device)
	}
	if device := env.Backend.Device("testnet-i"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("invite device close calls = %+v, want 1", device)
	}
}

// TestSetNetworkEnabled_StartsAndReportsStatus verifies that enabling a
// network starts it and returns its running status.
func TestSetNetworkEnabled_StartsAndReportsStatus(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)

	status, err := env.Runtime.SetNetworkEnabled("testnet", true)
	if err != nil {
		t.Fatalf("set network enabled: %v", err)
	}
	if !status.Enabled || !status.Running {
		t.Fatalf("status = %+v, want enabled and running", status)
	}
	if status.Reason != "" {
		t.Fatalf("reason = %q, want empty", status.Reason)
	}
	if env.Backend.Device("testnet") == nil || env.Backend.Device("testnet-i") == nil {
		t.Fatal("expected both plane devices")
	}
}

// TestSetNetworkEnabled_ReportsDivergence verifies that enabling a
// network that cannot start returns the recorded intent alongside why it
// is not running, rather than failing the operation.
func TestSetNetworkEnabled_ReportsDivergence(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, time.Hour)

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

// TestSetNetworkEnabled_DisableStopsNetwork verifies that disabling a
// running network stops it and returns the stopped status.
func TestSetNetworkEnabled_DisableStopsNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	status, err := env.Runtime.SetNetworkEnabled("testnet", false)
	if err != nil {
		t.Fatalf("set network enabled: %v", err)
	}
	if status.Enabled || status.Running {
		t.Fatalf("status = %+v, want disabled and stopped", status)
	}
	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("main device close calls = %+v, want 1", device)
	}
	if device := env.Backend.Device("testnet-i"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("invite device close calls = %+v, want 1", device)
	}
}

func TestStop_StopsEverything(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
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
		t.Fatalf("main device close calls = %+v, want 1", device)
	}
	if device := env.Backend.Device("testnet-i"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("invite device close calls = %+v, want 1", device)
	}
}

func TestStop_RetiresReconcileTimer(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	// A registration expiring shortly arms the timer well inside this
	// test's lifetime, so a timer surviving the stop would be observed.
	// Expiry is persisted in whole seconds, so one second is the floor.
	expiry := time.Second
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"soon",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5"), ExpiresIn: &expiry},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	env.Converge(t, "testnet")

	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatalf("disable network: %v", err)
	}
	env.Converge(t, "testnet")

	// Wipe the observed peer state: any further reconciliation would have
	// to apply the peer set again, which the mock records.
	env.Backend.Device("testnet").SetPeers()
	applied := len(env.Backend.AppliedOpsFor("testnet"))

	time.Sleep(expiry + 500*time.Millisecond)

	if got := len(env.Backend.AppliedOpsFor("testnet")); got != applied {
		t.Fatalf("peer ops after stop = %d, want %d — the reconcile timer outlived the network", got, applied)
	}
}

// newRuntime builds a runtime over env with a custom tick interval and
// mock WireGuard backend.
func newRuntime(
	t *testing.T,
	env *testutil.ServiceEnv,
	backend *wireguardtest.MockBackend,
	interval time.Duration,
) *runtime.Runtime {
	t.Helper()

	rt, err := runtime.New(runtime.Options{
		Service:   env.Service,
		WireGuard: wireguard.NewManagerWithBackend(backend),
		Wake:      env.Wake,
		Interval:  interval,
		Clock:     func() time.Time { return testutil.FixedTime },
		Logger:    logging.Discard(),
	})
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

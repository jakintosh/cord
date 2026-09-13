package runtime_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestReconcile_StartsEnabledNetworks(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	if err := testutil.ApplyNetwork(t, env.Runtime, "testnet"); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	network := networkStatus(t, env.Runtime, "testnet")
	if !network.Enabled || !network.Running {
		t.Fatalf("status = %+v, want enabled and running", network)
	}
	if network.Reason != "" {
		t.Fatalf("reason = %q, want empty", network.Reason)
	}
	if !network.Status.Reconcile.LastAttemptAt.Equal(testutil.FixedTime) {
		t.Fatalf("last reconcile attempt = %v, want %v", network.Status.Reconcile.LastAttemptAt, testutil.FixedTime)
	}
	if !network.Status.Reconcile.LastSuccessAt.Equal(testutil.FixedTime) {
		t.Fatalf("last reconcile success = %v, want %v", network.Status.Reconcile.LastSuccessAt, testutil.FixedTime)
	}
	if env.Backend.Device("testnet") == nil || env.Backend.Device("testnet-i") == nil {
		t.Fatal("expected both plane devices")
	}
}

func TestReconcile_LeavesDisabledNetworksStopped(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)

	if err := testutil.ApplyNetwork(t, env.Runtime, "testnet"); err != nil {
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

func TestReconcileNetwork_FailedStartKeepsIntentAndReportsReason(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, time.Hour)

	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	if err := testutil.ApplyNetwork(t, rt, "testnet"); err == nil {
		t.Fatal("reconcileNetwork should fail while the device cannot be created")
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
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, 5*time.Millisecond)

	if _, err := rt.SetNetworkEnabled(t.Context(), "testnet", true); err != nil {
		t.Fatalf("reconcile = %v, want device failure", err)
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

func TestReconcileNetwork_DisableStopsRunningNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	if !networkStatus(t, env.Runtime, "testnet").Running {
		t.Fatal("network should be running")
	}

	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatalf("disable network: %v", err)
	}
	env.ReconcileNetwork(t, "testnet")

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

func TestReconcileNetwork_RecordsFailureAndRecovery(t *testing.T) {
	now := testutil.FixedTime
	var clock atomic.Value
	clock.Store(now)
	env := testutil.SetupRuntimeWithClock(t, func() time.Time { return clock.Load().(time.Time) })
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	main := env.Backend.Device("testnet")
	now = now.Add(time.Minute)
	clock.Store(now)
	main.PeersErr = errors.New("read peers failed")
	if err := testutil.ApplyNetwork(t, env.Runtime, "testnet"); !(err != nil && strings.Contains(err.Error(), main.PeersErr.Error())) {
		t.Fatalf("reconcileNetwork = %v, want peer read failure", err)
	}

	status := networkStatus(t, env.Runtime, "testnet")
	if status.Health != runtime.HealthDegraded {
		t.Fatalf("health = %q, want degraded", status.Health)
	}
	if status.Status.Reconcile.Error == "" {
		t.Fatal("reconcile error should describe the current failure")
	}
	if !status.Status.Reconcile.LastAttemptAt.Equal(now) {
		t.Fatalf("last attempt = %v, want %v", status.Status.Reconcile.LastAttemptAt, now)
	}
	if !status.Status.Reconcile.LastSuccessAt.Equal(testutil.FixedTime) {
		t.Fatalf("last success = %v, want %v", status.Status.Reconcile.LastSuccessAt, testutil.FixedTime)
	}
	overall, err := env.Runtime.GetOverview(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if overall.Health != runtime.HealthDegraded {
		t.Fatalf("overall health = %q, want degraded", overall.Health)
	}

	now = now.Add(time.Minute)
	clock.Store(now)
	main.PeersErr = nil
	env.ReconcileNetwork(t, "testnet")

	status = networkStatus(t, env.Runtime, "testnet")
	if status.Health != runtime.HealthHealthy {
		t.Fatalf("health = %q, want healthy", status.Health)
	}
	if status.Status.Reconcile.Error != "" {
		t.Fatalf("reconcile error = %q, want cleared", status.Status.Reconcile.Error)
	}
	if !status.Status.Reconcile.LastSuccessAt.Equal(now) {
		t.Fatalf("last success = %v, want %v", status.Status.Reconcile.LastSuccessAt, now)
	}
	overall, err = env.Runtime.GetOverview(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if overall.Health != runtime.HealthHealthy {
		t.Fatalf("overall health = %q, want healthy", overall.Health)
	}
}

// TestSetNetworkEnabled_StartsAndReportsStatus verifies that enabling a
// network starts it and returns its running status.
func TestSetNetworkEnabled_StartsAndReportsStatus(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)

	status, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", true)
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

	status, err := rt.SetNetworkEnabled(t.Context(), "testnet", true)
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

	status, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", false)
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

	if err := testutil.ApplyNetwork(t, env.Runtime, "testnet"); err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	env.Stop()

	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("main device close calls = %+v, want 1", device)
	}
	if device := env.Backend.Device("testnet-i"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("invite device close calls = %+v, want 1", device)
	}
}

func TestReconcile_SkipsDisabledNetwork(t *testing.T) {
	synctest.Test(t, testReconcileSkipsDisabledNetwork)
}

func testReconcileSkipsDisabledNetwork(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatalf("disable network: %v", err)
	}
	env.ReconcileNetwork(t, "testnet")

	// Wipe the observed peer state: any further reconciliation would have
	// to apply the peer set again, which the mock records.
	env.Backend.Device("testnet").SetPeers()
	applied := len(env.Backend.AppliedOpsFor("testnet"))

	// A periodic pass must not apply peers to the disabled network.
	time.Sleep(runtime.DefaultInterval + time.Second)
	synctest.Wait()

	if got := len(env.Backend.AppliedOpsFor("testnet")); got != applied {
		t.Fatalf("peer ops after disable = %d, want %d", got, applied)
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
		Service:     env.Service,
		WireGuard:   wireguard.NewManagerWithBackend(backend),
		Interval:    interval,
		Clock:       func() time.Time { return testutil.FixedTime },
		Logger:      logging.Discard(),
		DNSDisabled: true,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	testutil.RunRuntime(t, rt, runtime.Handlers{})
	return rt
}

// networkStatus returns the status of one network, failing the test when
// the runtime does not know it.
func networkStatus(
	t *testing.T,
	rt *runtime.Runtime,
	name string,
) runtime.NetworkOverview {
	t.Helper()

	status, err := rt.GetOverview(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, network := range status.Networks {
		if network.Name == name {
			return network
		}
	}
	t.Fatalf("network %q missing from status", name)
	return runtime.NetworkOverview{}
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

func TestRunOwnsOneLifetime(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}

	backend := wireguardtest.NewMockBackend()
	rt := newLifecycleRuntime(t, env.Service, backend)
	if len(backend.CreateCalls) != 0 {
		t.Fatal("construction started devices")
	}

	stop := testutil.RunRuntime(t, rt, runtime.Handlers{})
	first := backend.Device("testnet")
	if first == nil {
		t.Fatal("Run did not apply saved intent")
	}
	if err := rt.Run(t.Context(), runtime.Handlers{}); !errors.Is(err, runtime.ErrAlreadyRun) {
		t.Fatalf("second Run = %v", err)
	}

	snapshot := testutil.RuntimeStatus(t, rt)
	delete(snapshot.Networks, "testnet")
	if len(testutil.RuntimeStatus(t, rt).Networks) != 1 {
		t.Fatal("changing the returned snapshot changed runtime state")
	}

	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if first.Check() == nil || backend.Device("testnet-i").Check() == nil {
		t.Fatal("Run returned before closing its devices")
	}
	if _, err := rt.ListNetworks(t.Context()); !errors.Is(err, runtime.ErrNotRunning) {
		t.Fatalf("command after shutdown = %v", err)
	}
	if err := rt.Run(t.Context(), runtime.Handlers{}); !errors.Is(err, runtime.ErrAlreadyRun) {
		t.Fatalf("reuse after shutdown = %v", err)
	}

	replacement := newLifecycleRuntime(t, env.Service, backend)
	testutil.RunRuntime(t, replacement, runtime.Handlers{})
	if backend.Device("testnet") == first {
		t.Fatal("a fresh runtime did not reconstruct saved intent")
	}
}

func TestCanceledRunDoesNotStartResources(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}

	backend := wireguardtest.NewMockBackend()
	rt := newLifecycleRuntime(t, env.Service, backend)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := rt.Run(ctx, runtime.Handlers{}); err != nil {
		t.Fatal(err)
	}
	if len(backend.CreateCalls) != 0 {
		t.Fatal("a canceled Run created devices")
	}
	if _, err := rt.Status(t.Context()); !errors.Is(err, runtime.ErrNotRunning) {
		t.Fatalf("status after canceled Run = %v", err)
	}
}

func TestRunReturnsCleanupFailure(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	main := env.Backend.Device("testnet")
	failure := errors.New("device close failed")
	main.CloseErr = failure
	if err := env.Stop(); !errors.Is(err, failure) {
		t.Fatalf("Run cleanup = %v", err)
	}
	if env.Backend.Device("testnet-i").Check() == nil {
		t.Fatal("one cleanup failure prevented independent cleanup")
	}
	main.CloseErr = nil
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedNetworkCleanupPreventsReplacement(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	main := env.Backend.Device("testnet")
	main.CloseErr = errors.New("device close failed")
	t.Cleanup(func() { main.CloseErr = nil })

	status, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", false)
	if err != nil || status.Reason == "" || !status.Running {
		t.Fatalf("failed disable = %+v, %v", status, err)
	}
	status, err = env.Runtime.SetNetworkEnabled(t.Context(), "testnet", true)
	if err != nil || status.Reason == "" || env.Backend.Device("testnet") != main {
		t.Fatalf("enable bypassed unfinished cleanup: %+v, %v", status, err)
	}

	main.CloseErr = nil
	status, err = env.Runtime.SetNetworkEnabled(t.Context(), "testnet", true)
	if err != nil || status.Health != runtime.HealthHealthy || env.Backend.Device("testnet") == main {
		t.Fatalf("cleanup retry = %+v, %v", status, err)
	}
}

func TestRuntimePrunesExpiredRegistrationsOnPeriodicPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := testutil.SetupRuntimeWithClock(t, time.Now)
		testutil.SeedNetwork(t, env.Service)
		env.Enable(t, "testnet")
		expiry := 2 * time.Second
		_, err := env.Runtime.CreateRegistration(t.Context(), "testnet", "short-lived", service.RegistrationOptions{
			PeerIP:    net.ParseIP("10.0.0.5"),
			ExpiresIn: &expiry,
		})
		if err != nil {
			t.Fatal(err)
		}

		invite := env.Backend.Device("testnet-i")
		if peers, err := invite.GetPeerStatuses(); err != nil || len(peers) != 1 {
			t.Fatalf("invite peers before expiry = %+v, %v", peers, err)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if peers, err := invite.GetPeerStatuses(); err != nil || len(peers) != 1 {
			t.Fatalf("invite peers before periodic cleanup = %+v, %v", peers, err)
		}

		// Frequent commands must not postpone the periodic cleanup.
		for range int(runtime.DefaultInterval / time.Second) {
			if _, err := env.Runtime.Status(t.Context()); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Second)
		}
		synctest.Wait()
		if peers, err := invite.GetPeerStatuses(); err != nil || len(peers) != 0 {
			t.Fatalf("invite peers after expiry = %+v, %v", peers, err)
		}
		if env.Backend.Device("testnet-i") != invite {
			t.Fatal("expiry replaced the device")
		}
		status := testutil.RuntimeStatus(t, env.Runtime)
		if status.Networks["testnet"].Reconcile.Interval != runtime.DefaultInterval {
			t.Fatalf("network maintenance interval = %v", status.Networks["testnet"].Reconcile.Interval)
		}

		if err := env.Stop(); err != nil {
			t.Fatal(err)
		}
		creates := len(env.Backend.CreateCalls)
		time.Sleep(time.Hour)
		synctest.Wait()
		if len(env.Backend.CreateCalls) != creates {
			t.Fatal("maintenance outlived Run")
		}
	})
}

func TestRuntimeSnapshotSurvivesStorageFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := testutil.SetupService(t)
		testutil.SeedNetwork(t, env.Service)
		if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
			t.Fatal(err)
		}
		store := &overviewStore{Store: env.Database}
		svc, err := service.New(service.Options{Store: store})
		if err != nil {
			t.Fatal(err)
		}
		backend := wireguardtest.NewMockBackend()
		rt := newLifecycleRuntime(t, svc, backend)
		testutil.RunRuntime(t, rt, runtime.Handlers{})
		device := backend.Device("testnet")
		failure := errors.New("database unavailable")
		store.setError(failure)
		time.Sleep(time.Hour)
		synctest.Wait()
		before := testutil.RuntimeStatus(t, rt)
		if before.Resource.State != resource.Running || before.Reconcile.Error == "" || before.Networks["testnet"].Main.WireGuard.State != resource.Running {
			t.Fatalf("maintenance failure changed availability: %+v", before)
		}
		if _, err := rt.GetOverview(t.Context()); !errors.Is(err, failure) {
			t.Fatalf("overview = %v, want storage error", err)
		}
		if backend.Device("testnet") != device {
			t.Fatal("storage failure replaced existing resources")
		}

		store.setError(nil)
		if err := testutil.ApplyNetwork(t, rt, "testnet"); err != nil {
			t.Fatal(err)
		}
		if testutil.RuntimeStatus(t, rt).Reconcile != before.Reconcile {
			t.Fatal("targeted success erased the failed full pass")
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		if status := testutil.RuntimeStatus(t, rt); status.Reconcile.Error != "" || status.Resource.State != resource.Running {
			t.Fatalf("recovered status = %+v", status)
		}
	})
}

func TestRuntimeFullPassContainsAndJoinsChildFailures(t *testing.T) {
	env := testutil.SetupService(t)
	for _, name := range []string{"one", "two", "healthy"} {
		if _, err := env.Service.CreateNetwork(name, "192.168.1.1",
			service.Plane{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 8080},
			service.Plane{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 8080},
		); err != nil {
			t.Fatal(err)
		}
		if err := env.Service.SetNetworkEnabled(name, true); err != nil {
			t.Fatal(err)
		}
	}
	first, second := errors.New("first device failed"), errors.New("second device failed")
	backend := &selectiveBackend{
		MockBackend: wireguardtest.NewMockBackend(),
		failures: map[string]error{
			"one": &resource.Error{Recovery: resource.Fatal, Cause: first},
			"two": second,
		},
	}
	rt := newLifecycleRuntime(t, env.Service, backend)
	testutil.RunRuntime(t, rt, runtime.Handlers{})
	status := testutil.RuntimeStatus(t, rt)
	if !strings.Contains(status.Reconcile.Error, first.Error()) || !strings.Contains(status.Reconcile.Error, second.Error()) {
		t.Fatalf("full pass lost child failures: %+v", status)
	}

	if status := testutil.RuntimeStatus(t, rt); status.Resource.State != resource.Running || status.Networks["healthy"].Main.WireGuard.State != resource.Running {
		t.Fatalf("failed child prevented healthy network startup: %+v", status)
	}
}

func TestRuntimeCleansDeletedNetworkAfterFailedStop(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	main := env.Backend.Device("testnet")
	main.CloseErr = errors.New("close failed")
	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatal(err)
	}
	if err := testutil.ApplyNetwork(t, env.Runtime, "testnet"); !(err != nil && strings.Contains(err.Error(), main.CloseErr.Error())) {
		t.Fatal(err)
	}
	if err := env.Service.DeleteNetwork("testnet"); err != nil {
		t.Fatal(err)
	}
	main.CloseErr = nil
	if err := env.Stop(); err != nil {
		t.Fatal(err)
	}
	if main.Check() == nil {
		t.Fatal("deleted network's retained resources were not cleaned up")
	}
}

func newLifecycleRuntime(
	t *testing.T,
	svc *service.Service,
	backend wireguard.Backend,
) *runtime.Runtime {
	t.Helper()
	rt, err := runtime.New(runtime.Options{
		Service: svc, WireGuard: wireguard.NewManagerWithBackend(backend),
		DNSDisabled: true, Interval: time.Hour,
		Clock: func() time.Time { return testutil.FixedTime },
	})
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

type overviewStore struct {
	service.Store
	mu      sync.Mutex
	listErr error
}

func (s *overviewStore) ListNetworks() ([]*service.Network, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Store.ListNetworks()
}

func (s *overviewStore) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listErr = err
}

type selectiveBackend struct {
	*wireguardtest.MockBackend
	failures map[string]error
}

func (b *selectiveBackend) CreateDevice(config wireguard.DeviceConfig) (wireguard.WgDevice, error) {
	if err := b.failures[config.Name]; err != nil {
		return nil, err
	}
	return b.MockBackend.CreateDevice(config)
}

// A request canceled while waiting to submit a command must not persist later.
func TestCanceledQueuedCommandDoesNotPersist(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	store := &blockedIntentStore{Store: env.Database, entered: make(chan struct{}), release: make(chan struct{})}
	svc, err := service.New(service.Options{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	rt := newLifecycleRuntime(t, svc, wireguardtest.NewMockBackend())
	testutil.RunRuntime(t, rt, runtime.Handlers{})
	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	t.Cleanup(release)
	enabled := make(chan error, 1)
	go func() { _, err := rt.SetNetworkEnabled(t.Context(), "testnet", true); enabled <- err }()
	<-store.entered
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { _, err := rt.CreateGroup(ctx, "testnet", "canceled"); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued command = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled caller waited for the running command")
	}
	release()
	if err := <-enabled; err != nil {
		t.Fatal(err)
	}
	groups, err := rt.ListGroups(t.Context(), "testnet")
	if err != nil || len(groups) != 0 {
		t.Fatalf("queued command persisted: %+v, %v", groups, err)
	}
}

func TestCommandsExecuteSequentially(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		env := testutil.SetupService(t)
		testutil.SeedNetwork(t, env.Service)
		store := &blockedIntentStore{
			Store:   env.Database,
			entered: make(chan struct{}),
			release: make(chan struct{}),
		}
		svc, err := service.New(service.Options{Store: store})
		if err != nil {
			t.Fatal(err)
		}
		rt := newLifecycleRuntime(t, svc, wireguardtest.NewMockBackend())
		testutil.RunRuntime(t, rt, runtime.Handlers{})
		var once sync.Once
		release := func() { once.Do(func() { close(store.release) }) }
		t.Cleanup(release)

		first := make(chan error, 1)
		go func() {
			_, err := rt.SetNetworkEnabled(t.Context(), "testnet", true)
			first <- err
		}()
		<-store.entered

		second := make(chan error, 1)
		go func() {
			_, err := rt.CreateGroup(t.Context(), "testnet", "after-enable")
			second <- err
		}()
		synctest.Wait()
		groups, err := env.Service.ListGroups("testnet")
		if err != nil || len(groups) != 0 {
			t.Fatalf("second command overtook the first: %+v, %v", groups, err)
		}

		release()
		if err := <-first; err != nil {
			t.Fatal(err)
		}
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		groups, err = rt.ListGroups(t.Context(), "testnet")
		if err != nil || len(groups) != 1 {
			t.Fatalf("second command did not finish: %+v, %v", groups, err)
		}
	})
}

type blockedIntentStore struct {
	service.Store
	entered chan struct{}
	release chan struct{}
}

func (s *blockedIntentStore) SetNetworkEnabled(name string, enabled bool) error {
	close(s.entered)
	<-s.release
	return s.Store.SetNetworkEnabled(name, enabled)
}

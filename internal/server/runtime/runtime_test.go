package runtime_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
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

func TestRun_StartsEnabledNetworks(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	rt := newStoppedRuntime(t, env.Service, backend, time.Hour)
	testutil.RunRuntime(t, rt, runtime.Handlers{})

	network := networkStatus(t, rt, "testnet")
	if !network.Enabled || network.Snapshot == nil {
		t.Fatalf("status = %+v, want enabled and running", network)
	}
	if network.Reason != "" {
		t.Fatalf("reason = %q, want empty", network.Reason)
	}
	if !network.Reconcile.LastAttemptAt.Equal(testutil.FixedTime) {
		t.Fatalf("last reconcile attempt = %v, want %v", network.Reconcile.LastAttemptAt, testutil.FixedTime)
	}
	if !network.Reconcile.LastSuccessAt.Equal(testutil.FixedTime) {
		t.Fatalf("last reconcile success = %v, want %v", network.Reconcile.LastSuccessAt, testutil.FixedTime)
	}
	if backend.Device("testnet") == nil || backend.Device("testnet-i") == nil {
		t.Fatal("expected both plane devices")
	}
}

func TestRun_RetriesFailedStartOnTick(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := startRuntimeWithInterval(t, env, backend, 5*time.Millisecond)

	network := networkStatus(t, rt, "testnet")
	if network.Snapshot != nil || network.Reason == "" {
		t.Fatalf("status = %+v, want stopped with a reason", network)
	}

	// Reset clears the injected error under the backend's own lock.
	backend.Reset()

	waitFor(t, func() bool {
		network := networkStatus(t, rt, "testnet")
		return network.Snapshot != nil && network.Reason == ""
	}, "network to start on a later tick")
}

func TestRun_StopsEverything(
	t *testing.T,
) {
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

func TestRun_SkipsDisabledNetwork(
	t *testing.T,
) {
	synctest.Test(t, testReconcileSkipsDisabledNetwork)
}

func testReconcileSkipsDisabledNetwork(
	t *testing.T,
) {
	t.Helper()
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

// startRuntimeWithInterval builds a runtime over env with a custom tick interval and
// mock WireGuard backend.
func startRuntimeWithInterval(
	t *testing.T,
	env *testutil.ServiceEnv,
	backend *wireguardtest.MockBackend,
	interval time.Duration,
) *runtime.Runtime {
	t.Helper()

	rt := newStoppedRuntime(t, env.Service, backend, interval)
	testutil.RunRuntime(t, rt, runtime.Handlers{})
	return rt
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

func TestRun_OwnsOneLifetime(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}

	backend := wireguardtest.NewMockBackend()
	rt := newStoppedRuntime(t, env.Service, backend, time.Hour)
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

	replacement := newStoppedRuntime(t, env.Service, backend, time.Hour)
	testutil.RunRuntime(t, replacement, runtime.Handlers{})
	if backend.Device("testnet") == first {
		t.Fatal("a fresh runtime did not reconstruct saved intent")
	}
}

func TestRun_CanceledDoesNotStartResources(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}

	backend := wireguardtest.NewMockBackend()
	rt := newStoppedRuntime(t, env.Service, backend, time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := rt.Run(ctx, runtime.Handlers{}); err != nil {
		t.Fatal(err)
	}
	if len(backend.CreateCalls) != 0 {
		t.Fatal("a canceled Run created devices")
	}
	if _, err := rt.GetStatus(t.Context()); !errors.Is(err, runtime.ErrNotRunning) {
		t.Fatalf("status after canceled Run = %v", err)
	}
}

func TestRun_ReturnsCleanupFailure(
	t *testing.T,
) {
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

func TestRun_PrunesExpiredRegistrationsOnPeriodicPass(
	t *testing.T,
) {
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
			if _, err := env.Runtime.GetStatus(t.Context()); err != nil {
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
		if resourceAt(t, status.Snapshot, "testnet").Activities["reconcile"].Interval != runtime.DefaultInterval {
			t.Fatalf("network maintenance interval = %v", resourceAt(t, status.Snapshot, "testnet").Activities["reconcile"].Interval)
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

func TestRun_SnapshotSurvivesStorageFailure(
	t *testing.T,
) {
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
		rt := newStoppedRuntime(t, svc, backend, time.Hour)
		testutil.RunRuntime(t, rt, runtime.Handlers{})
		device := backend.Device("testnet")
		failure := errors.New("database unavailable")
		store.setError(failure)
		time.Sleep(time.Hour)
		synctest.Wait()
		before := testutil.RuntimeStatus(t, rt)
		if before.Snapshot.State != resource.Running || before.Snapshot.Activities["reconcile"].Error == "" || resourceAt(t, before.Snapshot, "testnet", "main", "wireguard-device").State != resource.Running {
			t.Fatalf("maintenance failure changed availability: %+v", before)
		}
		if status, err := rt.GetStatus(t.Context()); err != nil || !strings.Contains(status.Snapshot.Error, failure.Error()) || status.Health != resource.HealthDegraded {
			t.Fatalf("status = %+v, %v, want snapshot with storage error", status, err)
		}
		if backend.Device("testnet") != device {
			t.Fatal("storage failure replaced existing resources")
		}

		store.setError(nil)
		if err := testutil.ApplyNetwork(t, rt, "testnet"); err != nil {
			t.Fatal(err)
		}
		if testutil.RuntimeStatus(t, rt).Snapshot.Activities["reconcile"] != before.Snapshot.Activities["reconcile"] {
			t.Fatal("targeted success erased the failed full pass")
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		if status := testutil.RuntimeStatus(t, rt); status.Snapshot.Activities["reconcile"].Error != "" || status.Snapshot.State != resource.Running {
			t.Fatalf("recovered status = %+v", status)
		}
	})
}

func TestRun_ContainsAndJoinsChildFailures(
	t *testing.T,
) {
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
	rt := newStoppedRuntime(t, env.Service, backend, time.Hour)
	testutil.RunRuntime(t, rt, runtime.Handlers{})
	status := testutil.RuntimeStatus(t, rt)
	if !strings.Contains(status.Snapshot.Activities["reconcile"].Error, first.Error()) || !strings.Contains(status.Snapshot.Activities["reconcile"].Error, second.Error()) {
		t.Fatalf("full pass lost child failures: %+v", status)
	}

	if status := testutil.RuntimeStatus(t, rt); status.Snapshot.State != resource.Running || resourceAt(t, status.Snapshot, "healthy", "main", "wireguard-device").State != resource.Running {
		t.Fatalf("failed child prevented healthy network startup: %+v", status)
	}
}

func TestRun_CleansDeletedNetworkAfterFailedStop(
	t *testing.T,
) {
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

func newStoppedRuntime(
	t *testing.T,
	svc *service.Service,
	backend wireguard.Backend,
	interval time.Duration,
) *runtime.Runtime {
	t.Helper()
	rt, err := runtime.New(runtime.Options{
		Service:     svc,
		WireGuard:   wireguard.NewManagerWithBackend(backend),
		DNSDisabled: true,
		Interval:    interval,
		Clock:       func() time.Time { return testutil.FixedTime },
		Logger:      logging.Discard(),
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

func (s *overviewStore) ListNetworks() (
	[]*service.Network,
	error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Store.ListNetworks()
}

func (s *overviewStore) setError(
	err error,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listErr = err
}

type selectiveBackend struct {
	*wireguardtest.MockBackend
	failures map[string]error
}

func (b *selectiveBackend) CreateDevice(
	config wireguard.DeviceConfig) (wireguard.WgDevice, error,
) {
	if err := b.failures[config.Name]; err != nil {
		return nil, err
	}
	return b.MockBackend.CreateDevice(config)
}

// A request canceled while waiting to submit a command must not persist later.
func TestExecute_CanceledQueuedCommandDoesNotPersist(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	store := &blockedIntentStore{Store: env.Database, entered: make(chan struct{}), release: make(chan struct{})}
	svc, err := service.New(service.Options{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	rt := newStoppedRuntime(t, svc, wireguardtest.NewMockBackend(), time.Hour)
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

func TestExecute_CommandsRunSequentially(
	t *testing.T,
) {
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
		rt := newStoppedRuntime(t, svc, wireguardtest.NewMockBackend(), time.Hour)
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

func (s *blockedIntentStore) SetNetworkEnabled(
	name string,
	enabled bool,
) error {
	close(s.entered)
	<-s.release
	return s.Store.SetNetworkEnabled(name, enabled)
}

package runtime_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestNew_RequiresDependencies(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	manager := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())
	tests := []struct {
		name string
		opts runtime.Options
	}{
		{name: "service", opts: runtime.Options{WireGuard: manager}},
		{name: "wireguard manager", opts: runtime.Options{Service: env.Service}},
		{name: "DNS manager", opts: runtime.Options{Service: env.Service, WireGuard: manager}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := runtime.New(tt.opts)
			if err == nil || rt != nil {
				t.Fatalf("new without %s = %v, %v; want nil runtime and error", tt.name, rt, err)
			}
		})
	}
}

func TestRun_RetainsPartialDeviceUntilCleanup(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}
	backend := &partialStartupBackend{MockBackend: wireguardtest.NewMockBackend()}
	rt, err := runtime.New(runtime.Options{
		Service:    env.Service,
		WireGuard:  wireguard.NewManagerWithBackend(backend),
		DNS:        testutil.NewDNSManager(t),
		HTTPClient: testutil.NewOfflineHTTPClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.RunRuntime(t, rt)
	if status := networkStatus(t, rt, "testnet"); status.Reason == "" {
		t.Fatal("expected partial startup failure")
	}
	device := backend.Device("testnet")
	t.Cleanup(func() { device.CloseErr = nil })
	status := networkStatus(t, rt, "testnet")
	if !status.Enabled || status.Resource == nil || status.Reason == "" {
		t.Fatalf("partial startup status = %+v", status)
	}
	if err := rt.Converge(t.Context(), "testnet"); err == nil {
		t.Fatal("expected cleanup failure")
	}
	if len(backend.CreateCalls) != 1 {
		t.Fatal("partial device was replaced before cleanup")
	}
	device.CloseErr = nil
	if err := rt.Converge(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	if device.CloseCalls != 3 || len(backend.CreateCalls) != 2 {
		t.Fatal("partial device cleanup was not retried before replacement")
	}
	status = networkStatus(t, rt, "testnet")
	if status.Resource == nil || status.Reason != "" {
		t.Fatalf("recovered startup status = %+v", status)
	}
}

// partialStartupBackend leaves a handle behind on the first failed creation.
type partialStartupBackend struct {
	*wireguardtest.MockBackend
}

func (b *partialStartupBackend) CreateDevice(
	config wireguard.DeviceConfig,
) (
	wireguard.WgDevice,
	error,
) {
	device, err := b.MockBackend.CreateDevice(config)
	if err != nil {
		return nil, err
	}
	if len(b.CreateCalls) == 1 {
		b.Device(config.Name).CloseErr = errors.New("partial cleanup failed")
		return device, errors.New("partial creation failed")
	}
	return device, nil
}

func TestRun_StartsEnabledNetworks(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{HTTPClient: testutil.NewOfflineHTTPClient()})
	testutil.RunRuntime(t, rt)

	network := networkStatus(t, rt, "testnet")
	if !network.Enabled || network.Resource == nil {
		t.Fatalf("status = %+v, want enabled and running", network)
	}
	if network.Reason != "" {
		t.Fatalf("reason = %q, want empty", network.Reason)
	}
	if backend.Device("testnet") == nil {
		t.Fatal("expected the network device")
	}
}

func TestRun_LeavesDisabledNetworksStopped(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	network := networkStatus(t, env.Runtime, "testnet")
	if network.Enabled || network.Resource != nil {
		t.Fatalf("status = %+v, want disabled and stopped", network)
	}
	if network.Health != resource.HealthInactive {
		t.Fatalf("health = %q, want inactive", network.Health)
	}
	if env.Backend.Device("testnet") != nil {
		t.Fatal("disabled network should not create a device")
	}
}

func TestRun_RetriesFailedStartOnTick(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, runtime.Options{
		Interval:   5 * time.Millisecond,
		HTTPClient: testutil.NewOfflineHTTPClient(),
	})
	testutil.RunRuntime(t, rt)

	waitFor(t, func() bool {
		return networkStatus(t, rt, "testnet").Reason != ""
	}, "initial startup failure")

	network := networkStatus(t, rt, "testnet")
	if network.Resource != nil || network.Reason == "" {
		t.Fatalf("status = %+v, want stopped with a reason", network)
	}

	// Reset clears the injected error under the backend's own lock.
	backend.Reset()

	waitFor(t, func() bool {
		network := networkStatus(t, rt, "testnet")
		return network.Resource != nil && network.Reason == ""
	}, "network to start on a later tick")
}

func TestRun_ClosesNetworksBeforeReturning(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	finish := env.Stop
	waitFor(t, func() bool {
		return networkStatus(t, env.Runtime, "testnet").Resource != nil
	}, "initial network startup")
	if err := finish(); err != nil {
		t.Fatal(err)
	}

	if _, err := env.Runtime.GetStatus(t.Context()); !errors.Is(err, runtime.ErrNotRunning) {
		t.Fatalf("status after shutdown = %v", err)
	}
	record, err := env.Service.GetNetwork("testnet")
	if err != nil || !record.Enabled {
		t.Fatalf("saved intent after shutdown = %+v, %v", record, err)
	}
	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
}

func TestRun_RejectsConcurrentRunAndReuse(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	finish := env.Stop
	waitFor(t, func() bool {
		return networkStatus(t, env.Runtime, "testnet").Resource != nil
	}, "initial network startup")
	if err := env.Runtime.Run(t.Context()); !errors.Is(err, runtime.ErrAlreadyRun) {
		t.Fatalf("concurrent Run = %v", err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	if err := env.Runtime.Run(t.Context()); !errors.Is(err, runtime.ErrAlreadyRun) {
		t.Fatalf("second Run = %v", err)
	}
}

func TestRun_CanceledBeforeStartup(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{})
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := rt.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(backend.CreateCalls) != 0 {
		t.Fatal("canceled runtime started a device")
	}
}

func TestCommands_WaitForRunAndRespectCancellation(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := rt.SetNetworkEnabled(ctx, "testnet", true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command before Run = %v", err)
	}
	if len(backend.CreateCalls) != 0 {
		t.Fatal("command created a device before Run")
	}

	// An uncanceled command can wait for Run to become available.
	result := make(chan error, 1)
	go func() {
		_, err := rt.GetStatus(t.Context())
		result <- err
	}()
	testutil.RunRuntime(t, rt)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting command was not released by Run")
	}
	status := networkStatus(t, rt, "testnet")
	if status.Enabled || status.Resource != nil {
		t.Fatalf("canceled command changed intent: %+v", status)
	}
}

func TestRun_InitialFailureReleasesWaitingCommands(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{})
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(t.Context()); err == nil {
		t.Fatal("expected initial storage failure")
	}
	if _, err := rt.GetStatus(t.Context()); !errors.Is(err, runtime.ErrNotRunning) {
		t.Fatalf("status after startup failure = %v", err)
	}
}

func TestRun_ReturnsCleanupError(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	device := env.Backend.Device("testnet")
	cleanupErr := errors.New("device cleanup failed")
	device.CloseErr = cleanupErr
	t.Cleanup(func() {
		device.CloseErr = nil
		_ = device.Close()
	})
	if err := env.Stop(); !errors.Is(err, cleanupErr) {
		t.Fatalf("shutdown = %v, want cleanup failure", err)
	}
	if device.CloseCalls != 1 {
		t.Fatalf("cleanup attempts = %d, want 1", device.CloseCalls)
	}
}

func TestRun_CancellationReleasesInFlightSync(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}
	transport := &blockingSyncTransport{started: make(chan struct{}, 1)}
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{
		HTTPClient: &http.Client{Transport: transport},
	})
	finish := testutil.RunRuntime(t, rt)
	select {
	case <-transport.started:
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not start")
	}
	// Scheduled work holds the same owner as public commands.
	commandCtx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := rt.GetStatus(commandCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("status during scheduled sync = %v, want deadline", err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	if backend.Device("testnet").CloseCalls != 1 {
		t.Fatal("Run returned without releasing the device")
	}
}

type blockingSyncTransport struct {
	started chan struct{}
}

func (t *blockingSyncTransport) RoundTrip(
	request *http.Request,
) (
	*http.Response,
	error,
) {
	t.started <- struct{}{}
	<-request.Context().Done()
	return nil, request.Context().Err()
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
	if opts.DNS == nil {
		opts.DNS = testutil.NewDNSManager(t)
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = testutil.NewOfflineHTTPClient()
	}
	opts.Logger = logging.Discard()
	if opts.Clock == nil {
		opts.Clock = func() time.Time { return testutil.FixedTime }
	}

	rt, err := runtime.New(opts)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
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

package runtime_test

import (
	"context"
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

func TestLookupNetwork_ResolvesInstalledPendingAndMissing(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "installed")
	testutil.SeedInstall(t, env.Service, "pending")

	lookup, err := env.Runtime.LookupNetwork(t.Context(), "installed")
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Network == nil || lookup.Network.Name != "installed" || lookup.Install != nil {
		t.Fatalf("installed lookup = %+v", lookup)
	}

	lookup, err = env.Runtime.LookupNetwork(t.Context(), "pending")
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Install == nil || lookup.Install.Name != "pending" || lookup.Network != nil {
		t.Fatalf("pending lookup = %+v", lookup)
	}

	lookup, err = env.Runtime.LookupNetwork(t.Context(), "missing")
	if !errors.Is(err, service.ErrNotFound) || lookup.Network != nil || lookup.Install != nil {
		t.Fatalf("missing lookup = %+v, %v", lookup, err)
	}

	status, err := env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Networks) != 1 || len(status.Installs) != 1 || status.Installs[0].Name != "pending" {
		t.Fatalf("status = %+v", status)
	}
}

func TestLookupNetwork_ReportsStorageFailure(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedInstall(t, env.Service, "pending")
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}

	lookup, err := env.Runtime.LookupNetwork(t.Context(), "pending")
	if err == nil || errors.Is(err, service.ErrNotFound) || lookup.Network != nil || lookup.Install != nil {
		t.Fatalf("lookup = %+v, %v, want storage failure without a result", lookup, err)
	}
}

func TestSetNetworkEnabled_RetriesCleanup(
	t *testing.T,
) {
	env, device := networkWithBlockedCleanup(t)
	status, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", false)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Reason == "" || networkStatus(t, env.Runtime, "testnet").Resource == nil {
		t.Fatalf("failed cleanup status = %+v", status)
	}
	finishNetworkCleanup(t, env, device)
	observed := networkStatus(t, env.Runtime, "testnet")
	if observed.Resource != nil || observed.Reason != "" {
		t.Fatalf("completed cleanup status = %+v", observed)
	}
}

func TestUninstallNetwork_RetriesCleanup(
	t *testing.T,
) {
	env, device := networkWithBlockedCleanup(t)
	if err := env.Runtime.UninstallNetwork(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	finishNetworkCleanup(t, env, device)
	if _, err := env.Service.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("uninstalled network = %v", err)
	}
}

func networkWithBlockedCleanup(
	t *testing.T,
) (*testutil.RuntimeEnv, *wireguardtest.MockDevice) {
	t.Helper()
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	device := env.Backend.Device("testnet")
	device.CloseErr = errors.New("close failed")
	t.Cleanup(func() { device.CloseErr = nil })
	return env, device
}

func finishNetworkCleanup(
	t *testing.T,
	env *testutil.RuntimeEnv,
	device *wireguardtest.MockDevice,
) {
	t.Helper()
	if device.CloseCalls != 1 {
		t.Fatalf("close calls = %d, want 1", device.CloseCalls)
	}
	device.CloseErr = nil
	if err := env.Runtime.ConvergeAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if device.CloseCalls != 2 || len(env.Backend.CreateCalls) != 1 {
		t.Fatal("cleanup did not retry the original device exclusively")
	}
}

func TestUpdateNetwork_WaitsForCleanupAfterConfigurationReverts(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	record := testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	device := env.Backend.Device("testnet")
	device.CloseErr = errors.New("close failed")
	t.Cleanup(func() { device.CloseErr = nil })

	port := uint16(51820)
	if _, err := env.Runtime.UpdateNetwork(t.Context(), "testnet", service.NetworkOptions{ListenPort: &port}); err != nil {
		t.Fatal(err)
	}
	status := networkStatus(t, env.Runtime, "testnet")
	if status.Reason == "" || status.Health != runtime.HealthDegraded {
		t.Fatalf("failed replacement status = %+v", status)
	}
	if len(env.Backend.CreateCalls) != 1 {
		t.Fatal("replacement started before cleanup succeeded")
	}

	// Returning to the original configuration must not revive an instance
	// whose activities were already canceled by the first cleanup attempt.
	port = record.ListenPort
	if _, err := env.Runtime.UpdateNetwork(t.Context(), "testnet", service.NetworkOptions{ListenPort: &port}); err != nil {
		t.Fatal(err)
	}
	if device.CloseCalls != 2 || len(env.Backend.CreateCalls) != 1 {
		t.Fatal("reverted configuration did not retry cleanup before replacement")
	}
	device.CloseErr = nil
	env.Converge(t, "testnet")
	if device.CloseCalls != 3 || len(env.Backend.CreateCalls) != 2 {
		t.Fatal("successful cleanup did not allow replacement")
	}
	status = networkStatus(t, env.Runtime, "testnet")
	if status.Resource == nil || status.Reason != "" {
		t.Fatalf("replacement status = %+v", status)
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
	if network.Health != runtime.HealthInactive {
		t.Fatalf("health = %q, want inactive", network.Health)
	}
	if env.Backend.Device("testnet") != nil {
		t.Fatal("disabled network should not create a device")
	}
}

func TestConverge_FailedStartKeepsIntentAndReportsReason(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, runtime.Options{
		Interval:   time.Hour,
		HTTPClient: testutil.NewOfflineHTTPClient(),
	})
	testutil.RunRuntime(t, rt)

	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	if err := rt.Converge(t.Context(), "testnet"); err == nil {
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
	if !network.Enabled || network.Resource != nil {
		t.Fatalf("status = %+v, want enabled and not running", network)
	}
	if network.Reason == "" {
		t.Fatal("status reason should explain the divergence")
	}
	if network.Health != runtime.HealthDegraded {
		t.Fatalf("health = %q, want degraded", network.Health)
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

func TestConverge_DisableStopsRunningNetwork(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	if networkStatus(t, env.Runtime, "testnet").Resource == nil {
		t.Fatal("network should be running")
	}

	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatalf("disable network: %v", err)
	}
	env.Converge(t, "testnet")

	network := networkStatus(t, env.Runtime, "testnet")
	if network.Enabled || network.Resource != nil {
		t.Fatalf("status = %+v, want disabled and stopped", network)
	}
	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
}

func TestConverge_RestartsChangedConfiguration(
	t *testing.T,
) {
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

	if networkStatus(t, env.Runtime, "testnet").Resource == nil {
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

func TestConverge_StopsUninstalledNetwork(
	t *testing.T,
) {
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
	status, err := env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Networks) != 0 {
		t.Fatalf("status networks = %d, want 0", len(status.Networks))
	}
}

func TestUpdateNetwork_RestartsRunningNetwork(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")
	original := env.Backend.Device("testnet")

	port := uint16(51820)
	network, err := env.Runtime.UpdateNetwork(
		t.Context(),
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

func TestSetNetworkEnabled_ReportsDivergence(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("no such device")
	rt := newRuntime(t, env, backend, runtime.Options{
		HTTPClient: testutil.NewOfflineHTTPClient(),
	})
	testutil.RunRuntime(t, rt)

	status, err := rt.SetNetworkEnabled(t.Context(), "testnet", true)
	if err != nil {
		t.Fatalf("set network enabled: %v", err)
	}
	if !status.Enabled || networkStatus(t, rt, "testnet").Resource != nil {
		t.Fatalf("status = %+v, want enabled and not running", status)
	}
	if status.Reason == "" {
		t.Fatal("status reason should explain the divergence")
	}
}

func TestUninstallNetwork_StopsRunningNetwork(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	if err := env.Runtime.UninstallNetwork(t.Context(), "testnet"); err != nil {
		t.Fatalf("uninstall network: %v", err)
	}

	if device := env.Backend.Device("testnet"); device == nil || device.CloseCalls != 1 {
		t.Fatalf("device close calls = %+v, want 1", device)
	}
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

func TestSetNetworkEnabled_OutlivesCommandContext(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	ctx, cancel := context.WithCancel(t.Context())
	if _, err := env.Runtime.SetNetworkEnabled(ctx, "testnet", true); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	status := networkStatus(t, env.Runtime, "testnet")
	if status.Resource == nil || env.Backend.Device("testnet").Check() != nil {
		t.Fatal("command cancellation stopped the network")
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

func TestSync_RejectsUnavailableNetworks(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "disabled")
	testutil.SeedInstall(t, env.Service, "pending")

	for _, tc := range []struct {
		name string
		want error
	}{
		{name: "missing", want: service.ErrNotFound},
		{name: "pending", want: service.ErrNetworkNotEnabled},
		{name: "disabled", want: service.ErrNetworkNotEnabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			network, err := env.Runtime.Sync(t.Context(), tc.name)
			if network != nil || !errors.Is(err, tc.want) {
				t.Fatalf("sync = %v, %v; want nil, %v", network, err, tc.want)
			}
		})
	}
}

func TestSync_ReportsStorageFailure(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}

	network, err := env.Runtime.Sync(t.Context(), "missing")
	if network != nil || err == nil || errors.Is(err, service.ErrNotFound) || errors.Is(err, service.ErrNetworkNotEnabled) {
		t.Fatalf("sync = %v, %v; want storage failure", network, err)
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

type networkObservation struct {
	runtime.NetworkStatus
	Resource *runtime.ResourceSnapshot
}

// networkStatus returns the summary and owned resource of one network,
// failing the test when the runtime does not know it.
func networkStatus(
	t *testing.T,
	rt *runtime.Runtime,
	name string,
) networkObservation {
	t.Helper()

	status, err := rt.GetStatus(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, network := range status.Networks {
		if network.Name == name {
			observation := networkObservation{NetworkStatus: network}
			for _, child := range status.Snapshot.Children {
				if child.Kind == "network" && child.Name == name {
					observation.Resource = &child
					break
				}
			}
			return observation
		}
	}
	t.Fatalf("network %q missing from status", name)
	return networkObservation{}
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

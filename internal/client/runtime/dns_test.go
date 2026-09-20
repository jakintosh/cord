package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestInstall_DNSFailureKeepsTunnelAndSyncRunning(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	server := newInstallServer(t)
	applyErr := dns.ErrUnsupported
	var applied []dns.Config
	resolver := &resolverStub{
		apply: func(_ context.Context, config dns.Config) error {
			applied = append(applied, config)
			return applyErr
		},
	}
	rt := newRuntime(t, env, backend, runtime.Options{
		DNS:        resolver,
		HTTPClient: server.server.Client(),
		Interval:   time.Hour,
	})
	testutil.RunRuntime(t, rt)

	// Enrollment completes even though permanent-network DNS is unavailable.
	_, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatalf("DNS applications = %d, want only the permanent tunnel", len(applied))
	}
	config := applied[0]
	if config.Network != "testnet" || config.Domain != "testnet.internal" {
		t.Fatalf("DNS configuration = %+v", config)
	}
	device := backend.Device("testnet")
	if config.Interface != device.Name() || device.Check() != nil {
		t.Fatal("DNS failure lost the live permanent interface")
	}
	record, err := env.Service.GetNetwork("testnet")
	if err != nil {
		t.Fatal(err)
	}
	serverIP, _, err := net.ParseCIDR(record.Server.Route)
	if err != nil || !config.Nameserver.Equal(serverIP) {
		t.Fatalf("DNS server = %v, want route %s", config.Nameserver, record.Server.Route)
	}
	status := networkStatus(t, rt, "testnet")
	if status.Reason != "" || !strings.Contains(status.Reconcile.Error, applyErr.Error()) {
		t.Fatalf("contained DNS failure = %+v", status)
	}
	resolverState := resolverStatus(t, status.Resource)
	if resolverState.State != resource.Failed || resolverState.Activities["apply"].Error == "" {
		t.Fatalf("failed DNS resource = %+v", resolverState)
	}
	if _, err := rt.Sync(t.Context(), "testnet"); err != nil {
		t.Fatalf("sync with failed DNS: %v", err)
	}

	// Ordinary convergence retries DNS without replacing the healthy tunnel.
	applyErr = nil
	if err := rt.Converge(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	status = networkStatus(t, rt, "testnet")
	if status.Reconcile.Error != "" || status.Sync.LastSuccessAt.IsZero() {
		t.Fatalf("recovered network = %+v", status)
	}
	if len(applied) != 2 || backend.Device("testnet") != device {
		t.Fatal("DNS retry did not preserve the original tunnel")
	}
	resolverState = resolverStatus(t, status.Resource)
	if resolverState.State != resource.Running || resolverState.Health != runtime.HealthHealthy {
		t.Fatalf("recovered DNS resource = %+v", resolverState)
	}
}

func TestUninstallNetwork_RetainsDNSCleanupAfterRecordDeletion(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	cleanupErr := errors.New("resolver busy")
	var removed []dns.Config
	resolver := &resolverStub{
		apply: func(_ context.Context, config dns.Config) error {
			if config.Network == "testnet" {
				return errors.New("partially applied DNS")
			}
			return nil
		},
		remove: func(ctx context.Context, config dns.Config) error {
			if len(removed) == 0 && backend.Device(config.Network).Check() != nil {
				t.Error("tunnel closed before DNS removal")
			}
			if ctx.Err() != nil {
				t.Errorf("DNS removal context: %v", ctx.Err())
			}
			removed = append(removed, config)
			return cleanupErr
		},
	}
	rt := newRuntime(t, env, backend, runtime.Options{DNS: resolver, Interval: time.Hour})
	testutil.RunRuntime(t, rt)
	for _, name := range []string{"testnet", "other"} {
		testutil.SeedNetworkDirect(t, env.Database, name)
		if _, err := rt.SetNetworkEnabled(t.Context(), name, true); err != nil {
			t.Fatal(err)
		}
	}

	// Deletion persists and the tunnel closes despite failed partial-DNS cleanup.
	if err := rt.UninstallNetwork(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("uninstalled record = %v", err)
	}
	if backend.Device("testnet").Check() == nil {
		t.Fatal("DNS failure prevented tunnel closure")
	}
	status, err := rt.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var pending *runtime.ResourceSnapshot
	for _, child := range status.Snapshot.Children {
		if child.Name == "testnet" {
			pending = &child
		}
	}
	if pending == nil || pending.Activities["close"].Error == "" {
		t.Fatalf("pending DNS cleanup was forgotten: %+v", status.Snapshot)
	}
	resolverState := resolverStatus(t, pending)
	if resolverState.State != resource.Failed || resolverState.Target != resource.Stopped {
		t.Fatalf("retained DNS resource = %+v", resolverState)
	}
	if resolverState.Activities["close"].Error == "" {
		t.Fatal("DNS resource lost its cleanup failure")
	}
	if len(removed) != 1 || removed[0].Network != "testnet" {
		t.Fatalf("removed configurations = %+v", removed)
	}
	if backend.Device("other").Check() != nil {
		t.Fatal("uninstall affected the other network")
	}

	// A full pass still finds and retires the uninstalled owner.
	cleanupErr = nil
	if err := rt.ConvergeAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	all, err := rt.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Snapshot.Children) != 1 || all.Snapshot.Children[0].Name != "other" {
		t.Fatalf("resources after cleanup = %+v", all.Snapshot.Children)
	}
	if len(removed) != 2 || removed[1].Interface != removed[0].Interface {
		t.Fatalf("cleanup retry lost the attempted configuration: %+v", removed)
	}
}

func TestUpdateNetwork_WaitsForDNSCleanupBeforeReplacement(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	backend := wireguardtest.NewMockBackend()
	cleanupErr := errors.New("resolver busy")
	resolver := &resolverStub{
		remove: func(context.Context, dns.Config) error { return cleanupErr },
	}
	rt := newRuntime(t, env, backend, runtime.Options{DNS: resolver, Interval: time.Hour})
	testutil.RunRuntime(t, rt)
	if _, err := rt.SetNetworkEnabled(t.Context(), "testnet", true); err != nil {
		t.Fatal(err)
	}
	oldDevice := backend.Device("testnet")

	port := uint16(51825)
	_, err := rt.UpdateNetwork(t.Context(), "testnet", service.NetworkOptions{ListenPort: &port})
	if err != nil {
		t.Fatal(err)
	}
	if oldDevice.Check() == nil || len(backend.CreateCalls) != 1 {
		t.Fatal("replacement must close the tunnel and wait for DNS cleanup")
	}

	cleanupErr = nil
	if err := rt.Converge(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	if len(backend.CreateCalls) != 2 || backend.CreateCalls[1].ListenPort != port {
		t.Fatalf("replacement did not use saved configuration: %+v", backend.CreateCalls)
	}

	if _, err := rt.SetNetworkEnabled(t.Context(), "testnet", false); err != nil {
		t.Fatal(err)
	}
	if status := networkStatus(t, rt, "testnet"); status.Resource != nil {
		t.Fatalf("disabled network retained resources: %+v", status)
	}
}

func TestRun_DNSShutdownUsesFreshContextAndStillClosesTunnel(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	backend := wireguardtest.NewMockBackend()
	cleanupErr := errors.New("DNS cleanup failed")
	removed := false
	resolver := &resolverStub{
		remove: func(ctx context.Context, _ dns.Config) error {
			removed = true
			if ctx.Err() != nil {
				t.Errorf("shutdown passed canceled context: %v", ctx.Err())
			}
			if _, bounded := ctx.Deadline(); !bounded {
				t.Error("shutdown DNS cleanup has no deadline")
			}
			if backend.Device("testnet").Check() != nil {
				t.Error("shutdown closed tunnel before removing DNS")
			}
			return cleanupErr
		},
	}
	rt := newRuntime(t, env, backend, runtime.Options{DNS: resolver, Interval: time.Hour})
	stop := testutil.RunRuntime(t, rt)
	if _, err := rt.SetNetworkEnabled(t.Context(), "testnet", true); err != nil {
		t.Fatal(err)
	}

	if err := stop(); !errors.Is(err, cleanupErr) {
		t.Fatalf("shutdown = %v, want DNS cleanup failure", err)
	}
	if !removed || backend.Device("testnet").Check() == nil {
		t.Fatal("shutdown did not attempt both DNS removal and tunnel closure")
	}
}

func TestConverge_DNSFollowsRecreatedInterfaceIdentity(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	backend := &renamingBackend{MockBackend: wireguardtest.NewMockBackend()}
	var calls []string
	cleanupErr := errors.New("old resolver busy")
	resolver := &resolverStub{
		apply: func(_ context.Context, config dns.Config) error {
			calls = append(calls, "apply "+config.Interface)
			return nil
		},
		remove: func(_ context.Context, config dns.Config) error {
			calls = append(calls, "remove "+config.Interface)
			return cleanupErr
		},
	}
	rt, err := runtime.New(runtime.Options{
		Service:    env.Service,
		WireGuard:  wireguard.NewManagerWithBackend(backend),
		DNS:        resolver,
		HTTPClient: testutil.NewOfflineHTTPClient(),
		Interval:   time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.RunRuntime(t, rt)
	if _, err := rt.SetNetworkEnabled(t.Context(), "testnet", true); err != nil {
		t.Fatal(err)
	}

	// The saved record is unchanged, but repair assigns a different OS name.
	if err := backend.Device("testnet").Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Converge(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	want := []string{"apply utun1", "remove utun1"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("failed identity transition = %v, want %v", calls, want)
	}
	if backend.Device("testnet").Check() != nil {
		t.Fatal("old DNS cleanup failure closed the repaired tunnel")
	}

	cleanupErr = nil
	if err := rt.Converge(t.Context(), "testnet"); err != nil {
		t.Fatal(err)
	}
	want = append(want, "remove utun1", "apply utun2")
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("identity transition = %v, want %v", calls, want)
	}
}

func TestConvergeAll_RetriesStaleDNSCleanupFromSavedEnabledNetworks(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	for _, name := range []string{"active", "disabled"} {
		testutil.SeedNetworkDirect(t, env.Database, name)
	}
	if err := env.Service.SetNetworkEnabled("active", true); err != nil {
		t.Fatal(err)
	}
	cleanupErr := errors.New("resolver directory unavailable")
	var activeSets []map[string]bool
	resolver := &resolverStub{
		cleanup: func(_ context.Context, active map[string]bool) error {
			activeSets = append(activeSets, active)
			if len(activeSets) == 1 && len(backend.CreateCalls) != 0 {
				t.Error("initial DNS cleanup ran after tunnel startup")
			}
			return cleanupErr
		},
	}
	rt := newRuntime(t, env, backend, runtime.Options{DNS: resolver, Interval: time.Hour})
	testutil.RunRuntime(t, rt)
	status, err := rt.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wantActive := map[string]bool{"active.internal": true}
	if len(activeSets) != 1 || !reflect.DeepEqual(activeSets[0], wantActive) {
		t.Fatalf("initial active domains = %v", activeSets)
	}
	if status.Snapshot.Activities["dns-cleanup"].Error == "" {
		t.Fatal("stale cleanup failure was not recorded")
	}
	if backend.Device("active") == nil {
		t.Fatal("stale cleanup failure blocked enabled network startup")
	}

	// Retry reads current intent, including changes since the initial failure.
	if _, err := rt.SetNetworkEnabled(t.Context(), "active", false); err != nil {
		t.Fatal(err)
	}
	cleanupErr = nil
	if err := rt.ConvergeAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(activeSets) != 2 || len(activeSets[1]) != 0 {
		t.Fatalf("retried active domains = %v", activeSets)
	}
	if err := rt.ConvergeAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(activeSets) != 2 {
		t.Fatal("successful startup cleanup was repeated")
	}
}

func TestRun_DoesNotCleanDNSWhenSavedStateCannotBeRead(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	called := false
	resolver := &resolverStub{
		cleanup: func(context.Context, map[string]bool) error {
			called = true
			return nil
		},
	}
	rt := newRuntime(t, env, wireguardtest.NewMockBackend(), runtime.Options{DNS: resolver})
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(t.Context()); err == nil {
		t.Fatal("expected saved-state read failure")
	}
	if called {
		t.Fatal("failed saved-state read authorized resolver deletion")
	}
}

func resolverStatus(
	t *testing.T,
	network *runtime.ResourceSnapshot,
) runtime.ResourceSnapshot {
	t.Helper()
	if network == nil {
		t.Fatal("network resource is missing")
	}
	for _, child := range network.Children {
		if child.Kind == "dns-resolver" {
			return child
		}
	}
	t.Fatal("DNS resource is missing")
	return runtime.ResourceSnapshot{}
}

// Callbacks run in the owner loop. Tests inspect or change their state only
// across completed runtime commands, with periodic DNS maintenance deferred.
type resolverStub struct {
	apply   func(context.Context, dns.Config) error
	remove  func(context.Context, dns.Config) error
	cleanup func(context.Context, map[string]bool) error
}

func (s *resolverStub) Apply(
	ctx context.Context,
	config dns.Config,
) error {
	if s.apply != nil {
		return s.apply(ctx, config)
	}
	return nil
}

func (s *resolverStub) Remove(
	ctx context.Context,
	config dns.Config,
) error {
	if s.remove != nil {
		return s.remove(ctx, config)
	}
	return nil
}

func (s *resolverStub) CleanupStale(
	ctx context.Context,
	active map[string]bool,
) error {
	if s.cleanup != nil {
		return s.cleanup(ctx, active)
	}
	return nil
}

type renamingBackend struct {
	*wireguardtest.MockBackend
}

func (b *renamingBackend) CreateDevice(
	config wireguard.DeviceConfig,
) (wireguard.WgDevice, error) {
	device, err := b.MockBackend.CreateDevice(config)
	if err != nil {
		return nil, err
	}
	return &renamedDevice{
		WgDevice: device,
		name:     fmt.Sprintf("utun%d", len(b.CreateCalls)),
	}, nil
}

type renamedDevice struct {
	wireguard.WgDevice
	name string
}

func (d *renamedDevice) Name() string {
	return d.name
}

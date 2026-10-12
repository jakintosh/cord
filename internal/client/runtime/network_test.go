package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestConverge_RepairsDeviceAndAppliesCurrentCache(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	record := testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	// Lose the interface, then change the cache while repair is unavailable.
	oldDevice := env.Backend.Device("testnet")
	if err := oldDevice.Close(); err != nil {
		t.Fatal(err)
	}
	peerKey := mustGenKey(t)
	testutil.SeedPeers(t, env.Database, "testnet", service.Peer{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	})
	env.Backend.CreateErr = errors.New("device unavailable")
	env.Converge(t, "testnet")
	status := networkStatus(t, env.Runtime, "testnet")
	if !status.Enabled || status.Resource == nil || status.Reconcile.Error == "" {
		t.Fatalf("failed repair status = %+v", status)
	}

	env.Backend.CreateErr = nil
	env.Converge(t, "testnet")
	status = networkStatus(t, env.Runtime, "testnet")
	if status.Reconcile.Error != "" || status.Reconcile.LastSuccessAt.IsZero() {
		t.Fatalf("recovered status = %+v", status)
	}
	device := env.Backend.Device("testnet")
	if device == oldDevice {
		t.Fatal("reconciliation did not replace the failed interface")
	}
	peers, err := device.GetPeerStatuses()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{record.Server.PublicKey: true, peerKey: true}
	if len(peers) != len(want) {
		t.Fatalf("recovered peers = %+v", peers)
	}
	for _, peer := range peers {
		if !want[peer.PublicKey.String()] {
			t.Errorf("unexpected recovered peer %s", peer.PublicKey)
		}
	}
}

func TestConverge_AppliesCachedPeersSynchronously(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	peerKey := mustGenKey(t)
	testutil.SeedPeers(t, env.Database, "testnet", service.Peer{
		Name:      "alice",
		PublicKey: peerKey,
		Route:     "10.42.0.9/32",
	})

	env.Enable(t, "testnet")

	device := env.Backend.Device("testnet")
	if device == nil {
		t.Fatal("expected the network device")
	}

	var found bool
	for _, op := range device.AppliedOps() {
		if op.Target.PublicKey.String() != peerKey {
			continue
		}
		found = true
		if op.Target.PersistentKeepalive != runtime.PersistentKeepaliveInterval {
			t.Errorf(
				"keepalive = %v, want %v",
				op.Target.PersistentKeepalive,
				runtime.PersistentKeepaliveInterval,
			)
		}
	}
	if !found {
		t.Errorf("cached peer %q was not applied to the device", peerKey)
	}
}

func TestConverge_PinsTheServerPeer(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	network := testutil.SeedNetworkDirect(t, env.Database, "testnet")
	env.Enable(t, "testnet")

	ops := env.Backend.AppliedOpsFor("testnet")
	if len(ops) != 1 {
		t.Fatalf("peer ops = %d, want 1 (the server)", len(ops))
	}
	if got := ops[0].Target.PublicKey.String(); got != network.Server.PublicKey {
		t.Errorf("public key = %q, want the server key %q", got, network.Server.PublicKey)
	}
	if ops[0].Target.PersistentKeepalive != runtime.PersistentKeepaliveInterval {
		t.Errorf(
			"server keepalive = %v, want %v",
			ops[0].Target.PersistentKeepalive,
			runtime.PersistentKeepaliveInterval,
		)
	}
}

func TestConverge_UsesConfiguredListenPort(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")

	port := uint16(51820)
	if err := env.Service.UpdateNetwork(
		"testnet",
		service.NetworkOptions{ListenPort: &port},
	); err != nil {
		t.Fatalf("set listen port: %v", err)
	}
	env.Enable(t, "testnet")

	if len(env.Backend.CreateCalls) != 1 {
		t.Fatalf("device creates = %d, want 1", len(env.Backend.CreateCalls))
	}
	if got := env.Backend.CreateCalls[0].ListenPort; got != port {
		t.Errorf("listen port = %d, want %d", got, port)
	}
}

func TestSync_AppliesServerSnapshot(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	server := newInstallServer(t)
	rt := newRuntime(t, env, backend, runtime.Options{HTTPClient: server.server.Client(),
		Interval:     time.Hour,
		SyncInterval: time.Hour,
	})
	testutil.RunRuntime(t, rt)

	if _, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{}); err != nil {
		t.Fatalf("install network: %v", err)
	}

	peerKey := mustGenKey(t)
	server.serve(protocol.VisiblePeer{
		Name:      "alice",
		Route:     "10.42.0.9/32",
		PublicKey: peerKey,
		Endpoints: []protocol.EndpointWitness{{
			Endpoint:  "5.6.7.8:51820",
			Timestamp: testutil.FixedTime,
		}},
	})

	network, err := rt.Sync(t.Context(), "testnet")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if network == nil || network.Name != "testnet" {
		t.Fatalf("synced network = %+v, want testnet record", network)
	}

	peers, err := env.Service.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("cached peers = %d, want 1", len(peers))
	}
	if peers[0].Name != "alice" {
		t.Errorf("cached peer name = %q, want alice", peers[0].Name)
	}

	var found bool
	for _, op := range backend.LastAppliedOpsFor("testnet") {
		if op.Target.PublicKey.String() == peerKey {
			found = true
		}
	}
	if !found {
		t.Errorf("synced peer %q was not applied to the device", peerKey)
	}

	status := networkStatus(t, rt, "testnet")
	if !status.Sync.LastAttemptAt.Equal(testutil.FixedTime) {
		t.Errorf("last sync attempt = %v, want %v", status.Sync.LastAttemptAt, testutil.FixedTime)
	}
	if !status.Sync.LastSuccessAt.Equal(testutil.FixedTime) {
		t.Errorf("last sync success = %v, want %v", status.Sync.LastSuccessAt, testutil.FixedTime)
	}
}

func TestActivities_RespectDueTimesAndOnDemandSync(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatal(err)
	}

	now := testutil.FixedTime
	var clock atomic.Value
	clock.Store(now)
	rt := newRuntime(t, env, wireguardtest.NewMockBackend(), runtime.Options{
		HTTPClient:     testutil.NewOfflineHTTPClient(),
		Interval:       time.Hour,
		SyncInterval:   2 * time.Minute,
		ScanInterval:   30 * time.Second,
		ReportInterval: time.Minute,
		Clock:          func() time.Time { return clock.Load().(time.Time) },
	})
	testutil.RunRuntime(t, rt)

	waitFor(t, func() bool {
		return !networkStatus(t, rt, "testnet").Sync.LastAttemptAt.IsZero()
	}, "immediate sync")
	status := networkStatus(t, rt, "testnet")
	if !status.Scan.LastAttemptAt.IsZero() || !status.Report.LastAttemptAt.IsZero() {
		t.Fatal("scan or report ran immediately at startup")
	}
	if status.Sync.Error == "" {
		t.Fatal("offline sync did not record its failure")
	}

	// Commands wake the loop after advancing the test clock. Status reads
	// remain projections; the activity timer drives the due work.
	now = now.Add(30 * time.Second)
	clock.Store(now)
	waitFor(t, func() bool {
		return networkStatus(t, rt, "testnet").Scan.LastAttemptAt.Equal(now)
	}, "first scan")
	status = networkStatus(t, rt, "testnet")
	if !status.Report.LastAttemptAt.IsZero() {
		t.Fatal("report ran before its first interval")
	}

	now = now.Add(30 * time.Second)
	clock.Store(now)
	waitFor(t, func() bool {
		return networkStatus(t, rt, "testnet").Report.LastAttemptAt.Equal(now)
	}, "first report")

	// A failed on-demand sync still postpones the scheduled attempt.
	if _, err := rt.Sync(t.Context(), "testnet"); err == nil {
		t.Fatal("offline on-demand sync unexpectedly succeeded")
	}
	lastSync := now
	now = now.Add(time.Minute)
	clock.Store(now)
	waitFor(t, func() bool {
		return networkStatus(t, rt, "testnet").Report.LastAttemptAt.Equal(now)
	}, "activities at the original sync deadline")
	status = networkStatus(t, rt, "testnet")
	if !status.Sync.LastAttemptAt.Equal(lastSync) {
		t.Fatal("scheduled sync ignored the on-demand attempt")
	}

	now = now.Add(time.Minute)
	clock.Store(now)
	waitFor(t, func() bool {
		return networkStatus(t, rt, "testnet").Sync.LastAttemptAt.Equal(now)
	}, "failed sync retry at its new deadline")
}

func TestSync_RecordsFailureAndRecovery(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	now := testutil.FixedTime
	var clock atomic.Value
	clock.Store(now)
	server := newInstallServer(t)
	rt := newRuntime(t, env, backend, runtime.Options{HTTPClient: server.server.Client(),
		Interval:     time.Hour,
		SyncInterval: time.Hour,
		Clock:        func() time.Time { return clock.Load().(time.Time) },
	})
	testutil.RunRuntime(t, rt)

	if _, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{}); err != nil {
		t.Fatalf("install network: %v", err)
	}
	waitFor(t, func() bool {
		return server.calls("snapshot") >= 1
	}, "initial sync to start")
	if _, err := rt.Sync(t.Context(), "testnet"); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	now = now.Add(time.Minute)
	clock.Store(now)
	server.setSnapshotFailure(true)
	if _, err := rt.Sync(t.Context(), "testnet"); err == nil {
		t.Fatal("sync unexpectedly succeeded")
	}

	status := networkStatus(t, rt, "testnet")
	if status.Health != resource.HealthDegraded {
		t.Fatalf("health = %q, want degraded", status.Health)
	}
	if status.Sync.Error == "" {
		t.Fatal("sync error should describe the current failure")
	}
	if !status.Sync.LastAttemptAt.Equal(now) {
		t.Fatalf("last attempt = %v, want %v", status.Sync.LastAttemptAt, now)
	}
	if !status.Sync.LastSuccessAt.Equal(testutil.FixedTime) {
		t.Fatalf("last success = %v, want %v", status.Sync.LastSuccessAt, testutil.FixedTime)
	}
	overall, err := rt.GetStatus(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if overall.Health != resource.HealthDegraded {
		t.Fatalf("overall health = %q, want degraded", overall.Health)
	}

	now = now.Add(time.Minute)
	clock.Store(now)
	server.setSnapshotFailure(false)
	if _, err := rt.Sync(t.Context(), "testnet"); err != nil {
		t.Fatalf("recovery sync: %v", err)
	}

	status = networkStatus(t, rt, "testnet")
	if status.Health != resource.HealthHealthy {
		t.Fatalf("health = %q, want healthy", status.Health)
	}
	if status.Sync.Error != "" {
		t.Fatalf("sync error = %q, want cleared", status.Sync.Error)
	}
	if !status.Sync.LastSuccessAt.Equal(now) {
		t.Fatalf("last success = %v, want %v", status.Sync.LastSuccessAt, now)
	}
	overall, err = rt.GetStatus(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if overall.Health != resource.HealthHealthy {
		t.Fatalf("overall health = %q, want healthy", overall.Health)
	}
}

func TestStop_RetiresScheduledActivities(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	server := newInstallServer(t)
	rt := newRuntime(t, env, backend, runtime.Options{HTTPClient: server.server.Client(),
		Interval:     time.Hour,
		SyncInterval: 5 * time.Millisecond,
		Clock:        time.Now,
	})
	finish := testutil.RunRuntime(t, rt)

	if _, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{}); err != nil {
		t.Fatalf("install network: %v", err)
	}
	waitFor(t, func() bool { return server.calls("snapshot") >= 2 }, "the next scheduled sync")

	if err := finish(); err != nil {
		t.Fatal(err)
	}
	syncs := server.calls("snapshot")

	time.Sleep(50 * time.Millisecond)

	if got := server.calls("snapshot"); got != syncs {
		t.Fatalf("syncs after stop = %d, want %d — scheduled work outlived the network", got, syncs)
	}
}

func TestGetNetworkTopology_ServerConnectivityUsesHandshake(
	t *testing.T,
) {
	tests := []struct {
		name          string
		includePeer   bool
		lastHandshake time.Time
		wantConnected bool
	}{
		{
			name:          "fresh handshake",
			includePeer:   true,
			lastHandshake: testutil.FixedTime,
			wantConnected: true,
		},
		{
			name:          "stale handshake",
			includePeer:   true,
			lastHandshake: testutil.FixedTime.Add(-2 * runtime.StaleThreshold),
			wantConnected: false,
		},
		{
			name:          "no handshake",
			includePeer:   true,
			wantConnected: false,
		},
		{
			name:          "missing peer",
			wantConnected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.SetupRuntime(t)
			network := testutil.SeedNetworkDirect(t, env.Database, "testnet")
			seedTopologyWithServer(t, env, "testnet")
			env.Enable(t, "testnet")

			if tt.includePeer {
				key, err := wgtypes.ParseKey(network.Server.PublicKey)
				if err != nil {
					t.Fatalf("parse server key: %v", err)
				}
				env.Backend.Device("testnet").SetPeers(wireguard.PeerStatus{
					PublicKey:     key,
					LastHandshake: tt.lastHandshake,
				})
			} else {
				env.Backend.Device("testnet").SetPeers()
			}

			result, err := env.Runtime.GetNetworkTopology(t.Context(), "testnet")
			if err != nil {
				t.Fatalf("network topology: %v", err)
			}
			got, ok := result.Connected["cord-server"]
			if !ok {
				t.Fatal("server connectivity missing from topology")
			}
			if got != tt.wantConnected {
				t.Errorf("server connected = %t, want %t", got, tt.wantConnected)
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
	if status.Reason == "" || status.Health != resource.HealthDegraded {
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
	if network.Health != resource.HealthDegraded {
		t.Fatalf("health = %q, want degraded", network.Health)
	}
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

func seedTopologyWithServer(
	t *testing.T,
	env *testutil.RuntimeEnv,
	network string,
) {
	t.Helper()

	server, err := topology.CidrFromString(
		"cord-server",
		"10.42.0.1/32",
		true,
	)
	if err != nil {
		t.Fatalf("build server CIDR: %v", err)
	}
	self, err := topology.CidrFromString("self", "10.42.0.5/32", true)
	if err != nil {
		t.Fatalf("build subject CIDR: %v", err)
	}
	if err := env.Database.ApplyNetworkReconciliation(
		network,
		service.NetworkReconciliation{
			Topology: topology.View{
				SubjectPeer: "self",
				Nodes: []topology.ViewNode{
					{Cidr: server, PeerName: "cord-server"},
					{Cidr: self, PeerName: "self", Subject: true},
				},
			},
			GeneratedAt: testutil.FixedTime,
			ReceivedAt:  testutil.FixedTime,
			PruneBefore: testutil.FixedTime.Add(-service.EndpointTTL),
		},
	); err != nil {
		t.Fatalf("seed topology: %v", err)
	}
}

func mustGenKey(t *testing.T) string {
	t.Helper()
	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pub, err := wireguard.PublicKey(key)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	return pub
}

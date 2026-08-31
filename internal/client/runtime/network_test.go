package runtime_test

import (
	"net"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

// TestConverge_AppliesCachedPeersSynchronously verifies that the cached
// peer set is on the device by the time a converge returns, without
// waiting for a sync.
func TestConverge_AppliesCachedPeersSynchronously(t *testing.T) {
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

// TestConverge_PinsTheServerPeer verifies that a network with no cached
// peers still gets the one peer it always has.
func TestConverge_PinsTheServerPeer(t *testing.T) {
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

func TestConverge_UsesConfiguredListenPort(t *testing.T) {
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

// TestSync_AppliesServerSnapshot verifies the on-demand sync round trip:
// fetch, persist through the service, project onto the device.
func TestSync_AppliesServerSnapshot(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{
		Interval:     time.Hour,
		SyncInterval: time.Hour,
	})
	server := newInstallServer(t)

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

	network, err := rt.Sync("testnet")
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

func TestSync_RecordsFailureAndRecovery(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	now := testutil.FixedTime
	rt := newRuntime(t, env, backend, runtime.Options{
		Interval:     time.Hour,
		SyncInterval: time.Hour,
		Clock:        func() time.Time { return now },
	})
	server := newInstallServer(t)

	if _, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{}); err != nil {
		t.Fatalf("install network: %v", err)
	}
	waitFor(t, func() bool {
		return server.calls("snapshot") >= 1
	}, "initial sync to start")
	if _, err := rt.Sync("testnet"); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	now = now.Add(time.Minute)
	server.setSnapshotFailure(true)
	if _, err := rt.Sync("testnet"); err == nil {
		t.Fatal("sync unexpectedly succeeded")
	}

	status := networkStatus(t, rt, "testnet")
	if status.Health != runtime.HealthDegraded {
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
	overall, err := rt.GetStatus()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if overall.Health != runtime.HealthDegraded {
		t.Fatalf("overall health = %q, want degraded", overall.Health)
	}

	now = now.Add(time.Minute)
	server.setSnapshotFailure(false)
	if _, err := rt.Sync("testnet"); err != nil {
		t.Fatalf("recovery sync: %v", err)
	}

	status = networkStatus(t, rt, "testnet")
	if status.Health != runtime.HealthHealthy {
		t.Fatalf("health = %q, want healthy", status.Health)
	}
	if status.Sync.Error != "" {
		t.Fatalf("sync error = %q, want cleared", status.Sync.Error)
	}
	if !status.Sync.LastSuccessAt.Equal(now) {
		t.Fatalf("last success = %v, want %v", status.Sync.LastSuccessAt, now)
	}
	overall, err = rt.GetStatus()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if overall.Health != runtime.HealthHealthy {
		t.Fatalf("overall health = %q, want healthy", overall.Health)
	}
}

// TestStop_RetiresActivityTimers verifies that no activity outlives the
// network whose device it configures.
func TestStop_RetiresActivityTimers(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{
		Interval:     time.Hour,
		SyncInterval: 5 * time.Millisecond,
	})
	server := newInstallServer(t)

	if _, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{}); err != nil {
		t.Fatalf("install network: %v", err)
	}
	waitFor(t, func() bool { return server.calls("snapshot") >= 2 }, "the sync timer to rearm")

	rt.Stop()
	syncs := server.calls("snapshot")

	time.Sleep(50 * time.Millisecond)

	if got := server.calls("snapshot"); got != syncs {
		t.Fatalf("syncs after stop = %d, want %d — a timer outlived the network", got, syncs)
	}
}

func TestPeerStatus_NotRunning(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	testutil.SeedPeers(t, env.Database, "testnet", service.Peer{
		Name:      "alice",
		PublicKey: mustGenKey(t),
		Route:     "10.42.0.9/32",
	})

	statuses, err := env.Runtime.GetPeerStatus("testnet")
	if err != nil {
		t.Fatalf("peer status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("peer statuses = %d, want 1", len(statuses))
	}
	got := statuses[0]
	if got.Name != "alice" || got.Route != "10.42.0.9/32" {
		t.Fatalf("status = %+v, want the cached peer", got)
	}
	if got.Connected || got.Endpoint != "" || !got.LastHandshake.IsZero() {
		t.Fatalf("status = %+v, want zero-valued device fields", got)
	}
}

func TestPeerStatus_JoinsLiveDeviceState(t *testing.T) {
	tests := []struct {
		name          string
		lastHandshake time.Time
		wantConnected bool
	}{
		{
			name:          "fresh handshake",
			lastHandshake: testutil.FixedTime,
			wantConnected: true,
		},
		{
			name:          "stale handshake",
			lastHandshake: testutil.FixedTime.Add(-2 * runtime.StaleThreshold),
			wantConnected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.SetupRuntime(t)
			testutil.SeedNetworkDirect(t, env.Database, "testnet")

			peerKey := mustGenKey(t)
			testutil.SeedPeers(t, env.Database, "testnet", service.Peer{
				Name:      "alice",
				PublicKey: peerKey,
				Route:     "10.42.0.9/32",
			})
			env.Enable(t, "testnet")

			key, err := wgtypes.ParseKey(peerKey)
			if err != nil {
				t.Fatalf("parse key: %v", err)
			}
			env.Backend.Device("testnet").SetPeers(wireguard.PeerStatus{
				PublicKey:     key,
				Endpoint:      &net.UDPAddr{IP: net.ParseIP("5.6.7.8"), Port: 51820},
				LastHandshake: tt.lastHandshake,
			})

			statuses, err := env.Runtime.GetPeerStatus("testnet")
			if err != nil {
				t.Fatalf("peer status: %v", err)
			}
			if len(statuses) != 1 {
				t.Fatalf("peer statuses = %d, want 1", len(statuses))
			}
			got := statuses[0]
			if got.Endpoint != "5.6.7.8:51820" {
				t.Errorf("endpoint = %q, want 5.6.7.8:51820", got.Endpoint)
			}
			if !got.LastHandshake.Equal(tt.lastHandshake) {
				t.Errorf("last handshake = %v, want %v", got.LastHandshake, tt.lastHandshake)
			}
			if got.Connected != tt.wantConnected {
				t.Errorf("connected = %t, want %t", got.Connected, tt.wantConnected)
			}
		})
	}
}

func TestNetworkTopology_ServerConnectivityUsesHandshake(t *testing.T) {
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

			result, err := env.Runtime.GetNetworkTopology("testnet")
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

package runtime_test

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestConverge_AppliesNewRegistrationToInviteDevice(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	inv, err := env.Service.CreateRegistration(
		"testnet",
		"live-reg",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	env.Converge(t, "testnet")

	if env.Backend.Device("testnet-i") == nil {
		t.Fatal("expected invite device")
	}
	peers := env.Backend.LastAppliedOpsFor("testnet-i")
	if len(peers) != 1 {
		t.Fatalf("invite peers = %d, want 1", len(peers))
	}
	expectedPub, err := wireguard.PublicKey(inv.Peer.PrivateKey)
	if err != nil {
		t.Fatalf("derive expected pub key: %v", err)
	}
	if peers[0].Target.PublicKey.String() != expectedPub {
		t.Fatalf("public key = %q, want temp registration public key %q",
			peers[0].Target.PublicKey.String(), expectedPub)
	}
	if got := peers[0].Target.AllowedIPs; len(got) != 1 || got[0].String() != "10.1.0.2/32" {
		t.Fatalf("allowed IPs = %v, want [10.1.0.2/32]", got)
	}
}

func TestConverge_AppliesRedeemedPeerToMainDevice(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	if _, err := env.Service.CreateRegistration(
		"testnet",
		"live-redeem",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}

	permKey := mustGenKey(t)
	if _, err := env.Service.RedeemRegistration(
		"testnet",
		lastTempKey(t, env.Service, "testnet"),
		permKey,
	); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	env.Converge(t, "testnet")

	if peers := env.Backend.LastAppliedOpsFor("testnet-i"); len(peers) != 1 {
		t.Fatalf("invite peers after redeem = %d, want 1 (temp peer still active)", len(peers))
	}

	var found bool
	for _, op := range env.Backend.LastAppliedOpsFor("testnet") {
		if op.Target.PublicKey.String() == permKey {
			found = true
			if got := op.Target.AllowedIPs; len(got) != 1 || got[0].String() != "10.0.0.5/32" {
				t.Fatalf("redeemed peer allowed IPs = %v, want [10.0.0.5/32]", got)
			}
		}
	}
	if !found {
		t.Fatal("main device missing redeemed peer")
	}
}

func TestConverge_RetiresConfirmedRegistrationFromInviteDevice(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")

	if _, err := env.Service.CreateRegistration(
		"testnet",
		"alice",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	permKey := mustGenKey(t)
	if _, err := env.Service.RedeemRegistration("testnet", tempKey, permKey); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	env.Converge(t, "testnet")

	if hasPeerOp(env.Backend.LastAppliedOpsFor("testnet-i"), tempKey) {
		t.Error("invite device should not have confirmed registration peer")
	}
	if !hasPeerOp(env.Backend.LastAppliedOpsFor("testnet"), permKey) {
		t.Error("main device missing confirmed peer")
	}
}

func TestConverge_ObservesOnlyActiveMainPeerEndpoints(t *testing.T) {
	tests := []struct {
		name          string
		lastHandshake time.Time
		wantEndpoint  bool
	}{
		{
			name:          "active handshake",
			lastHandshake: testutil.FixedTime.Add(-wireguard.ActiveHandshakeThreshold + time.Second),
			wantEndpoint:  true,
		},
		{
			name:          "stale handshake",
			lastHandshake: testutil.FixedTime.Add(-wireguard.ActiveHandshakeThreshold),
			wantEndpoint:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.SetupRuntime(t)
			network := testutil.SeedNetwork(t, env.Service)
			env.Enable(t, "testnet")

			peerKey := mustGenKey(t)
			if _, err := env.Service.CreateRegistration(
				"testnet",
				"alice",
				service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
			); err != nil {
				t.Fatalf("create registration: %v", err)
			}
			tempKey := lastTempKey(t, env.Service, "testnet")
			if _, err := env.Service.RedeemRegistration("testnet", tempKey, peerKey); err != nil {
				t.Fatalf("redeem registration: %v", err)
			}

			key, err := wgtypes.ParseKey(peerKey)
			if err != nil {
				t.Fatalf("parse peer key: %v", err)
			}
			_, route, err := net.ParseCIDR("10.0.0.5/32")
			if err != nil {
				t.Fatalf("parse peer route: %v", err)
			}
			env.Backend.Device("testnet").SetPeers(wireguard.PeerStatus{
				PublicKey:     key,
				AllowedIPs:    []net.IPNet{*route},
				Endpoint:      &net.UDPAddr{IP: net.ParseIP("203.0.113.5"), Port: 51820},
				LastHandshake: tt.lastHandshake,
			})

			if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
				t.Fatalf("confirm peer: %v", err)
			}
			env.Converge(t, "testnet")

			endpoints, err := env.Database.GetRecentEndpoints(
				"testnet",
				testutil.FixedTime.Add(-time.Second),
			)
			if err != nil {
				t.Fatalf("get recent endpoints: %v", err)
			}
			witnesses := endpoints[peerKey]
			if tt.wantEndpoint {
				if len(witnesses) != 1 {
					t.Fatalf("endpoint witnesses = %d, want 1", len(witnesses))
				}
				if witnesses[0].Witness != network.PublicKey {
					t.Errorf("witness = %q, want server key %q", witnesses[0].Witness, network.PublicKey)
				}
				if witnesses[0].Endpoint != "203.0.113.5:51820" {
					t.Errorf("endpoint = %q, want 203.0.113.5:51820", witnesses[0].Endpoint)
				}
			} else if len(witnesses) != 0 {
				t.Errorf("endpoint witnesses = %d, want 0", len(witnesses))
			}
		})
	}
}

func TestNetwork_DNS_StartsNetworkHealthy(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	port := freeDNSPort(t)
	rt := newDNSRuntime(t, env, port)
	createLoopbackNetwork(t, env, "dnstest")

	if err := env.Service.SetNetworkEnabled("dnstest", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	if err := rt.Converge("dnstest"); err != nil {
		t.Fatalf("converge: %v", err)
	}

	status := networkStatus(t, rt, "dnstest")
	if !status.Running {
		t.Fatalf("status = %+v, want running", status)
	}
	if status.Health != runtime.HealthHealthy {
		t.Fatalf("health = %q, want healthy", status.Health)
	}
	if status.Reason != "" {
		t.Fatalf("reason = %q, want empty", status.Reason)
	}
	if status.Status.Main.DNS.Error != nil {
		t.Fatalf("dns status error = %q, want empty", status.Status.Main.DNS.Error)
	}
}

func TestNetwork_DNS_StartFailureRecordsReasonAndRecovers(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	// The test itself holds the DNS port, so the network's DNS bind
	// fails and the start rolls back.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold dns port: %v", err)
	}
	port := uint16(blocker.Addr().(*net.TCPAddr).Port)

	rt := newDNSRuntime(t, env, port)
	createLoopbackNetwork(t, env, "dnstest")

	if err := env.Service.SetNetworkEnabled("dnstest", true); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	if err := rt.Converge("dnstest"); err == nil {
		t.Fatal("converge should fail while the dns port is held")
	}

	status := networkStatus(t, rt, "dnstest")
	if status.Running {
		t.Fatalf("status = %+v, want the failed start to roll back", status)
	}
	if !strings.Contains(status.Reason, "dns:") {
		t.Fatalf("reason = %q, want a dns: failure", status.Reason)
	}
	if status.Health != runtime.HealthDegraded {
		t.Fatalf("health = %q, want degraded", status.Health)
	}

	// Releasing the port makes the next convergence start the network.
	if err := blocker.Close(); err != nil {
		t.Fatalf("release dns port: %v", err)
	}
	if err := rt.Converge("dnstest"); err != nil {
		t.Fatalf("converge after release: %v", err)
	}

	status = networkStatus(t, rt, "dnstest")
	if !status.Running || status.Health != runtime.HealthHealthy {
		t.Fatalf("status = %+v, want running and healthy", status)
	}
	if status.Status.Main.DNS.Error != nil {
		t.Fatalf("dns status error = %q, want empty", status.Status.Main.DNS.Error)
	}
}

func TestConverge_RecoversDeviceBeforeApplyingPeers(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	original := env.Backend.Device("testnet-i")

	registration, err := env.Service.CreateRegistration(
		"testnet",
		"recover",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}

	env.Converge(t, "testnet")
	if status := networkStatus(t, env.Runtime, "testnet"); status.Status.Reconcile.Error != "" {
		t.Fatalf("recovery pass reported a peer application failure: %s", status.Status.Reconcile.Error)
	}

	replacement := env.Backend.Device("testnet-i")
	if replacement == original {
		t.Fatal("failed device was not replaced")
	}
	key, err := wireguard.PublicKey(registration.Peer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	peers, err := replacement.GetPeerStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].PublicKey.String() != key {
		t.Fatalf("new peer was not applied during the recovery pass: %+v", peers)
	}
}

func TestConverge_PeerStartupFailureRollsBack(t *testing.T) {
	for _, stage := range []string{"prune", "main peers", "invite peers"} {
		t.Run(stage, func(t *testing.T) {
			env := testutil.SetupService(t)
			testutil.SeedNetwork(t, env.Service)
			failure := errors.New("store unavailable")
			store := &maintenanceStore{Store: env.Database}
			switch stage {
			case "prune":
				store.pruneErr = failure
			case "main peers":
				store.mainErr = failure
			case "invite peers":
				store.inviteErr = failure
			}
			svc, err := service.New(service.Options{Store: store})
			if err != nil {
				t.Fatal(err)
			}
			env.Service = svc
			backend := wireguardtest.NewMockBackend()
			rt := newRuntime(t, env, backend, time.Hour)
			if err := svc.SetNetworkEnabled("testnet", true); err != nil {
				t.Fatal(err)
			}
			if err := rt.Converge("testnet"); !errors.Is(err, failure) {
				t.Fatalf("converge = %v, want store failure", err)
			}
			status := networkStatus(t, rt, "testnet")
			if !status.Enabled || status.Running || status.Status != nil || !strings.Contains(status.Reason, failure.Error()) {
				t.Fatalf("failed startup status = %+v", status)
			}
			for _, name := range []string{"testnet", "testnet-i"} {
				if device := backend.Device(name); device == nil || device.CloseCalls != 1 {
					t.Fatalf("%s was not cleaned up: %+v", name, device)
				}
			}
			store.pruneErr, store.mainErr, store.inviteErr = nil, nil, nil
			if err := rt.Converge("testnet"); err != nil {
				t.Fatal(err)
			}
			if status := networkStatus(t, rt, "testnet"); !status.Running || status.Health != runtime.HealthHealthy || status.Reason != "" {
				t.Fatalf("recovery status = %+v", status)
			}
		})
	}
}

func TestConverge_MainFailureStillUpdatesInvitePeers(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	registration, err := env.Service.CreateRegistration(
		"testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	)
	if err != nil {
		t.Fatal(err)
	}
	main := env.Backend.Device("testnet")
	invite := env.Backend.Device("testnet-i")
	main.PeersErr = errors.New("main peer read failed")
	invite.PeersErr = errors.New("invite peer read failed")
	if err := env.Runtime.Converge("testnet"); !errors.Is(err, main.PeersErr) || !errors.Is(err, invite.PeersErr) {
		t.Fatalf("converge did not return both failures: %v", err)
	}
	invite.PeersErr = nil
	if err := env.Runtime.Converge("testnet"); !errors.Is(err, main.PeersErr) {
		t.Fatalf("converge = %v, want main failure", err)
	}
	key, err := wireguard.PublicKey(registration.Peer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPeerOp(env.Backend.LastAppliedOpsFor("testnet-i"), key) {
		t.Fatal("main failure prevented applying the invite peer")
	}
	status := networkStatus(t, env.Runtime, "testnet")
	if !status.Running || status.Status == nil || status.Health != runtime.HealthDegraded {
		t.Fatalf("failed maintenance should keep network owned: %+v", status)
	}
	main.PeersErr = nil
	env.Converge(t, "testnet")
	if env.Backend.Device("testnet") != main || env.Backend.Device("testnet-i") != invite {
		t.Fatal("peer failure unnecessarily replaced plane devices")
	}
}

func TestGetStatus_ReportsDeviceFailureWithoutRepairing(t *testing.T) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	device := env.Backend.Device("testnet")
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		status := networkStatus(t, env.Runtime, "testnet")
		if !status.Running || status.Health != runtime.HealthDegraded || status.Status.Main.WireGuard.State != resource.Failed {
			t.Fatalf("device failure status = %+v", status)
		}
		if status.Status.Reconcile.Error != "" || env.Backend.Device("testnet") != device {
			t.Fatal("status read ran maintenance")
		}
	}
	env.Converge(t, "testnet")
	if status := networkStatus(t, env.Runtime, "testnet"); status.Health != runtime.HealthHealthy {
		t.Fatalf("recovery status = %+v", status)
	}
}

func mustGenKey(t *testing.T) string {
	t.Helper()
	k, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pub, err := wireguard.PublicKey(k)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	return pub
}

func lastTempKey(
	t *testing.T,
	svc *service.Service,
	network string,
) string {
	t.Helper()
	regs, err := svc.ListRegistrations(network)
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) == 0 {
		t.Fatal("no registrations found")
	}
	return regs[len(regs)-1].InvitePublicKey
}

func hasPeerOp(
	ops []wireguard.PeerOp,
	pubKey string,
) bool {
	for _, op := range ops {
		if !op.Remove && op.Target.PublicKey.String() == pubKey {
			return true
		}
	}
	return false
}

// freeDNSPort reserves an ephemeral TCP port and releases it, so a test
// can hand a specific bindable port to the DNS runtime.
func freeDNSPort(
	t *testing.T,
) uint16 {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}

	return uint16(port)
}

// newDNSRuntime builds a runtime with DNS serving enabled on the given
// port, over a loopback network the test creates itself.
func newDNSRuntime(
	t *testing.T,
	env *testutil.ServiceEnv,
	port uint16,
) *runtime.Runtime {
	t.Helper()

	rt, err := runtime.New(runtime.Options{
		Service:   env.Service,
		WireGuard: wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend()),
		Wake:      env.Wake,
		Clock:     func() time.Time { return testutil.FixedTime },
		Logger:    logging.Discard(),
		DNSPort:   port,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(rt.Stop)

	return rt
}

// createLoopbackNetwork persists a network whose server address is
// 127.0.0.1, the one address the test host can always bind.
func createLoopbackNetwork(
	t *testing.T,
	env *testutil.ServiceEnv,
	name string,
) *service.Network {
	t.Helper()

	network, err := env.Service.CreateNetwork(
		name,
		"192.168.1.1",
		service.Plane{Cidr: "127.0.0.0/24", WireguardPort: 51830, ApiPort: 8080},
		service.Plane{Cidr: "172.16.10.0/24", WireguardPort: 51831, ApiPort: 8080},
	)
	if err != nil {
		t.Fatalf("create network %s: %v", name, err)
	}

	return network
}

type maintenanceStore struct {
	service.Store
	pruneErr  error
	mainErr   error
	inviteErr error
}

func (s *maintenanceStore) PruneExpiredRegistrations(
	network string,
	now time.Time,
) error {
	if s.pruneErr != nil {
		return s.pruneErr
	}
	return s.Store.PruneExpiredRegistrations(network, now)
}

func (s *maintenanceStore) ListPeers(
	network string,
) (
	[]*service.Peer,
	error,
) {
	if s.mainErr != nil {
		return nil, s.mainErr
	}
	return s.Store.ListPeers(network)
}

func (s *maintenanceStore) ListActiveRegistrations(
	network string,
	now time.Time,
) (
	[]*service.Registration,
	error,
) {
	if s.inviteErr != nil {
		return nil, s.inviteErr
	}
	return s.Store.ListActiveRegistrations(network, now)
}

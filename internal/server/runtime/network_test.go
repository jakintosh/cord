package runtime_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
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
	if status.DNS.Error != "" {
		t.Fatalf("dns status error = %q, want empty", status.DNS.Error)
	}
	if !status.DNS.LastAttemptAt.Equal(testutil.FixedTime) {
		t.Fatalf("dns last attempt = %v, want %v", status.DNS.LastAttemptAt, testutil.FixedTime)
	}
	if !status.DNS.LastSuccessAt.Equal(testutil.FixedTime) {
		t.Fatalf("dns last success = %v, want %v", status.DNS.LastSuccessAt, testutil.FixedTime)
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
	if status.DNS.Error != "" {
		t.Fatalf("dns status error = %q, want empty", status.DNS.Error)
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

package runtime_test

import (
	"net"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
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

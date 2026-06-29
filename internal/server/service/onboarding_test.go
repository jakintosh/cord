package service_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// TestOnboardingLifecycle exercises the full server-side onboarding
// flow: create invite → redeem → confirm. It verifies the auth model
// at each step: a provisional peer can authenticate to /confirm but
// not to /peers, and a confirmed peer can authenticate to /peers.
func TestOnboardingLifecycle(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name:      "alice",
		IP:        net.ParseIP("10.0.0.5"),
		ExpiresIn: time.Hour,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	result, err := env.Service.RedeemInvite("testnet", tempKey, "alice-perm-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if result.AssignedCidr != "10.0.0.5/16" {
		t.Errorf("assigned_cidr = %q, want 10.0.0.5/16", result.AssignedCidr)
	}

	peer, err := env.Service.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer after redeem: %v", err)
	}
	if peer.Confirmed {
		t.Error("peer should not be confirmed after redeem")
	}
	if !peer.Enabled {
		t.Error("peer should be enabled after redeem")
	}

	provisional, err := env.Service.ResolveProvisionalIdentity("testnet", net.ParseIP("10.0.0.5"))
	if err != nil {
		t.Fatalf("resolve provisional identity: %v", err)
	}
	if provisional.Name != "alice" {
		t.Errorf("provisional name = %q, want alice", provisional.Name)
	}

	_, err = env.Service.ResolvePeerIdentity("testnet", net.ParseIP("10.0.0.5"))
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("resolve peer identity before confirm: err = %v, want ErrNotFound", err)
	}

	mainDev := env.WireGuard.Devices["testnet"]
	if mainDev == nil {
		t.Fatal("expected main device")
	}
	if !hasWGPeer(mainDev.AppliedPeers(), "alice-perm-key") {
		t.Error("main device missing provisional peer after redeem")
	}

	inviteDev := env.WireGuard.Devices["testnet-i"]
	if inviteDev == nil {
		t.Fatal("expected invite device")
	}
	if !hasWGPeer(inviteDev.AppliedPeers(), tempKey) {
		t.Error("invite device missing temp peer after redeem")
	}

	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	peer, err = env.Service.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer after confirm: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should be confirmed after confirm")
	}
	if !peer.Enabled {
		t.Error("peer should still be enabled after confirm")
	}

	confirmed, err := env.Service.ResolvePeerIdentity("testnet", net.ParseIP("10.0.0.5"))
	if err != nil {
		t.Fatalf("resolve peer identity after confirm: %v", err)
	}
	if confirmed.Name != "alice" {
		t.Errorf("confirmed name = %q, want alice", confirmed.Name)
	}

	_, err = env.Service.ResolveProvisionalIdentity("testnet", net.ParseIP("10.0.0.5"))
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("resolve provisional identity after confirm: err = %v, want ErrNotFound", err)
	}

	inviteDev = env.WireGuard.Devices["testnet-i"]
	if hasWGPeer(inviteDev.AppliedPeers(), tempKey) {
		t.Error("invite device should not have temp peer after confirm")
	}

	mainDev = env.WireGuard.Devices["testnet"]
	if !hasWGPeer(mainDev.AppliedPeers(), "alice-perm-key") {
		t.Error("main device missing confirmed peer after confirm")
	}
}

// TestConfirmPeer_PreservesDisabledState verifies that ConfirmPeer
// only flips the confirmed flag and does not re-enable a peer that
// an admin disabled between redeem and confirm.
func TestConfirmPeer_PreservesDisabledState(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "bob",
		IP:   net.ParseIP("10.0.0.6"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemInvite("testnet", tempKey, "bob-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	_, err = env.Service.UpdatePeer("testnet", "bob", service.UpdatePeerRequest{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("disable peer: %v", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "bob"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	peer, err := env.Service.GetPeer("testnet", "bob")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should be confirmed")
	}
	if peer.Enabled {
		t.Error("peer should still be disabled after confirm")
	}
}

// TestPruneExpiredInvites_RemovesExpiredProvisionalPeer verifies
// that an expired invite plus its orphaned provisional peer are
// pruned when reconciliation runs.
func TestPruneExpiredInvites_RemovesExpiredProvisionalPeer(t *testing.T) {
	now := testutil.FixedTime
	clock := &mutableClock{t: now}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name:      "short-lived",
		IP:        net.ParseIP("10.0.0.7"),
		ExpiresIn: time.Hour,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemInvite("testnet", tempKey, "short-lived-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	peer, err := env.Service.GetPeer("testnet", "short-lived")
	if err != nil {
		t.Fatalf("get peer before expiry: %v", err)
	}
	if peer.Confirmed {
		t.Fatal("peer should be provisional")
	}

	clock.t = clock.t.Add(2 * time.Hour)

	env.Service.Reconcile("testnet")

	_, err = env.Service.GetPeer("testnet", "short-lived")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("get peer after expiry+reconcile: err = %v, want ErrNotFound", err)
	}

	_, err = env.Database.GetInvite("testnet", "short-lived")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("get invite after expiry+reconcile: err = %v, want ErrNotFound", err)
	}

	mainDev := env.WireGuard.Devices["testnet"]
	if mainDev == nil {
		t.Fatal("expected main device")
	}
	if hasWGPeer(mainDev.AppliedPeers(), "short-lived-key") {
		t.Error("main device should not have expired provisional peer")
	}
}

// TestPruneExpiredInvites_RetainsActiveProvisionalPeer verifies
// that a provisional peer whose invite is still valid is not pruned.
func TestPruneExpiredInvites_RetainsActiveProvisionalPeer(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name:      "still-active",
		IP:        net.ParseIP("10.0.0.8"),
		ExpiresIn: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemInvite("testnet", tempKey, "still-active-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	env.Service.Reconcile("testnet")

	peer, err := env.Service.GetPeer("testnet", "still-active")
	if err != nil {
		t.Fatalf("get peer after reconcile: %v", err)
	}
	if peer.Confirmed {
		t.Error("peer should still be provisional")
	}
}

// TestPruneExpiredInvites_RetainsConfirmedPeerWithoutInvite verifies
// that a confirmed peer whose invite was deleted is not pruned.
func TestPruneExpiredInvites_RetainsConfirmedPeerWithoutInvite(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "confirmed",
		IP:   net.ParseIP("10.0.0.9"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemInvite("testnet", tempKey, "confirmed-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "confirmed"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if err := env.Service.RevokeInvite("testnet", "confirmed"); err != nil {
		t.Fatalf("revoke invite: %v", err)
	}

	env.Service.Reconcile("testnet")

	peer, err := env.Service.GetPeer("testnet", "confirmed")
	if err != nil {
		t.Fatalf("get peer after reconcile: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should still be confirmed")
	}
}

// TestPruneExpiredInvites_RetainsConfirmedInviteAsAudit verifies
// that a confirmed invite is not pruned even after its expiry passes.
func TestPruneExpiredInvites_RetainsConfirmedInviteAsAudit(t *testing.T) {
	now := testutil.FixedTime
	clock := &mutableClock{t: now}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name:      "audited",
		IP:        net.ParseIP("10.0.0.10"),
		ExpiresIn: time.Hour,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemInvite("testnet", tempKey, "audited-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "audited"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	clock.t = clock.t.Add(2 * time.Hour)

	env.Service.Reconcile("testnet")

	inv, err := env.Database.GetInvite("testnet", "audited")
	if err != nil {
		t.Fatalf("get invite after expiry+reconcile: %v", err)
	}
	if !inv.Confirmed {
		t.Error("invite should be confirmed")
	}
}

// --- helpers ---

type mutableClock struct {
	t time.Time
}

func (m *mutableClock) now() time.Time { return m.t }

func hasWGPeer(peers []wireguard.WGPeer, pubKey string) bool {
	for _, p := range peers {
		if p.PublicKey == pubKey {
			return true
		}
	}
	return false
}

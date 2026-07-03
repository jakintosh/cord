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

	alicePermKey := mustGenKey(t)
	aliceIP := net.ParseIP("10.0.0.5")
	expiresIn := time.Hour
	_, err := env.Service.CreateRegistration("testnet", "alice", &aliceIP, false, &expiresIn)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	result, err := env.Service.RedeemRegistration("testnet", tempKey, alicePermKey)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if result.Peer.CIDR != "10.0.0.5/16" {
		t.Errorf("cidr = %q, want 10.0.0.5/16", result.Peer.CIDR)
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

	if env.Backend.Device("testnet") == nil {
		t.Fatal("expected main device")
	}
	mainOps := env.Backend.AppliedOpsFor("testnet")
	if !hasPeerOp(mainOps, alicePermKey) {
		t.Error("main device missing provisional peer after redeem")
	}

	if env.Backend.Device("testnet-i") == nil {
		t.Fatal("expected invite device")
	}
	inviteOps := env.Backend.AppliedOpsFor("testnet-i")
	if !hasPeerOp(inviteOps, tempKey) {
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

	inviteOps = env.Backend.LastAppliedOpsFor("testnet-i")
	if hasPeerOp(inviteOps, tempKey) {
		t.Error("invite device should not have temp peer after confirm")
	}

	mainOps = env.Backend.LastAppliedOpsFor("testnet")
	if !hasPeerOp(mainOps, alicePermKey) {
		t.Error("main device missing confirmed peer after confirm")
	}
}

// TestConfirmPeer_PreservesDisabledState verifies that ConfirmPeer
// only flips the confirmed flag and does not re-enable a peer that
// an admin disabled between redeem and confirm.
func TestConfirmPeer_PreservesDisabledState(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	bobIP := net.ParseIP("10.0.0.6")
	_, err := env.Service.CreateRegistration("testnet", "bob", &bobIP, false, nil)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	bobKey := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey, bobKey)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	_, err = env.Service.UpdatePeer("testnet", "bob", nil, nil, &disabled, nil)
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

// TestPruneExpiredRegistrations_RemovesExpiredProvisionalPeer verifies
// that an expired registration plus its orphaned provisional peer are
// pruned when reconciliation runs.
func TestPruneExpiredRegistrations_RemovesExpiredProvisionalPeer(t *testing.T) {
	now := testutil.FixedTime
	clock := &mutableClock{t: now}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	shortIP := net.ParseIP("10.0.0.7")
	shortExpiry := time.Hour
	_, err := env.Service.CreateRegistration("testnet", "short-lived", &shortIP, false, &shortExpiry)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	shortKey := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey, shortKey)
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

	_, err = env.Database.GetRegistration("testnet", "short-lived")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("get registration after expiry+reconcile: err = %v, want ErrNotFound", err)
	}

	if env.Backend.Device("testnet") == nil {
		t.Fatal("expected main device")
	}
	mainOps := env.Backend.LastAppliedOpsFor("testnet")
	if hasPeerOp(mainOps, shortKey) {
		t.Error("main device should not have expired provisional peer")
	}
}

// TestPruneExpiredRegistrations_RetainsActiveProvisionalPeer verifies
// that a provisional peer whose registration is still valid is not
// pruned.
func TestPruneExpiredRegistrations_RetainsActiveProvisionalPeer(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	stillIP := net.ParseIP("10.0.0.8")
	stillExpiry := 24 * time.Hour
	_, err := env.Service.CreateRegistration("testnet", "still-active", &stillIP, false, &stillExpiry)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	stillKey := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey, stillKey)
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

// TestPruneExpiredRegistrations_RetainsConfirmedPeerWithoutInvite verifies
// that a confirmed peer whose registration was deleted is not pruned.
func TestPruneExpiredRegistrations_RetainsConfirmedPeerWithoutInvite(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	confirmedIP := net.ParseIP("10.0.0.9")
	_, err := env.Service.CreateRegistration("testnet", "confirmed", &confirmedIP, false, nil)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	confKey := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey, confKey)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "confirmed"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if err := env.Service.RevokeRegistration("testnet", "confirmed"); err != nil {
		t.Fatalf("revoke registration: %v", err)
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

// TestPruneExpiredRegistrations_RetainsConfirmedRegistrationAsAudit verifies
// that a confirmed registration is not pruned even after its expiry
// passes.
func TestPruneExpiredRegistrations_RetainsConfirmedRegistrationAsAudit(t *testing.T) {
	now := testutil.FixedTime
	clock := &mutableClock{t: now}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)

	auditedIP := net.ParseIP("10.0.0.10")
	auditedExpiry := time.Hour
	_, err := env.Service.CreateRegistration("testnet", "audited", &auditedIP, false, &auditedExpiry)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")

	auditKey := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey, auditKey)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "audited"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	clock.t = clock.t.Add(2 * time.Hour)

	env.Service.Reconcile("testnet")

	reg, err := env.Database.GetRegistration("testnet", "audited")
	if err != nil {
		t.Fatalf("get registration after expiry+reconcile: %v", err)
	}
	if !reg.Confirmed {
		t.Error("registration should be confirmed")
	}
}

// --- helpers ---

type mutableClock struct {
	t time.Time
}

func (m *mutableClock) now() time.Time { return m.t }

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

func hasPeerOp(ops []wireguard.PeerOp, pubKey string) bool {
	for _, op := range ops {
		if !op.Remove && op.Config.PublicKey.String() == pubKey {
			return true
		}
	}
	return false
}

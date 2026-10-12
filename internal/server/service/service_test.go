package service_test

import (
	"errors"
	"net"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestResolveRegistrationIdentity_RetainsRedeemedIdentityUntilConfirmation(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	inv, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatal(err)
	}
	inviteIP, _, err := net.ParseCIDR(inv.Peer.Route)
	if err != nil {
		t.Fatal(err)
	}
	before, err := env.Service.ResolveRegistrationIdentity("testnet", inviteIP)
	if err != nil {
		t.Fatal(err)
	}
	if before.Name != "alice" || before.Redeemed || before.MainRoute != "10.0.0.5/32" {
		t.Fatalf("initial identity = %+v", before)
	}
	if _, err := env.Service.RedeemRegistration("testnet", before.InvitePublicKey, mustGenKey(t)); err != nil {
		t.Fatal(err)
	}
	after, err := env.Service.ResolveRegistrationIdentity("testnet", inviteIP)
	if err != nil || !after.Redeemed || after.Name != "alice" {
		t.Fatalf("redeemed identity = %+v, %v", after, err)
	}
	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.ResolveRegistrationIdentity("testnet", inviteIP); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("confirmed invite identity: %v", err)
	}
}

func TestResolveRegistrationIdentity_ExpiresAtDeadline(
	t *testing.T,
) {
	clock := &mutableClock{t: testutil.FixedTime}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)
	inv, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatal(err)
	}
	inviteIP, _, err := net.ParseCIDR(inv.Peer.Route)
	if err != nil {
		t.Fatal(err)
	}
	before, err := env.Service.ResolveRegistrationIdentity("testnet", inviteIP)
	if err != nil {
		t.Fatal(err)
	}
	clock.t = before.ExpiresAt
	if _, err := env.Service.ResolveRegistrationIdentity("testnet", inviteIP); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expired identity: %v", err)
	}
	registrations, err := env.Service.ListRegistrations("testnet")
	if err != nil || len(registrations) != 1 || registrations[0].Name != "alice" {
		t.Fatalf("lookup removed registration: %+v, %v", registrations, err)
	}
}

func TestResolvePeerIdentity_TransitionsOnConfirmation(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	ip := net.ParseIP("10.0.0.5")

	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}

	provisional, err := env.Service.ResolveProvisionalIdentity("testnet", ip)
	if err != nil {
		t.Fatalf("resolve provisional identity: %v", err)
	}
	if provisional.Name != "alice" {
		t.Fatalf("provisional identity = %q, want alice", provisional.Name)
	}
	if _, err := env.Service.ResolvePeerIdentity("testnet", ip); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("resolve confirmed identity before confirmation: err = %v, want ErrNotFound", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm peer: %v", err)
	}
	confirmed, err := env.Service.ResolvePeerIdentity("testnet", ip)
	if err != nil {
		t.Fatalf("resolve confirmed identity: %v", err)
	}
	if confirmed.Name != "alice" {
		t.Fatalf("confirmed identity = %q, want alice", confirmed.Name)
	}
	if _, err := env.Service.ResolveProvisionalIdentity("testnet", ip); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("resolve provisional identity after confirmation: err = %v, want ErrNotFound", err)
	}
}

func TestResolvePeerIdentity_ExcludesDisabled(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	if _, err := env.Service.CreateRegistration("testnet", "toggle", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "toggle"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	disabled := false
	if _, err := env.Service.UpdatePeer("testnet", "toggle", service.PeerDiff{Enabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := env.Service.ResolvePeerIdentity("testnet", ip); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("resolve disabled peer: err = %v, want ErrNotFound", err)
	}

	enabled := true
	if _, err := env.Service.UpdatePeer("testnet", "toggle", service.PeerDiff{Enabled: &enabled}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, err := env.Service.ResolvePeerIdentity("testnet", ip); err != nil {
		t.Fatalf("resolve re-enabled peer: %v", err)
	}
}

func TestResolvePeerIdentity_NormalizesSourceIP(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	if _, err := env.Service.CreateRegistration("testnet", "mapped", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "mapped"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	mapped := net.ParseIP("::ffff:10.0.0.5")
	peer, err := env.Service.ResolvePeerIdentity("testnet", mapped)
	if err != nil {
		t.Fatalf("resolve IPv4-mapped source: %v", err)
	}
	if peer.Name != "mapped" {
		t.Fatalf("identity = %q, want mapped", peer.Name)
	}
}

// wantInvalidInput checks the public sentinel and the validation rule.
func wantInvalidInput(
	t *testing.T,
	err error,
	fragment string,
) {
	t.Helper()

	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), fragment) {
		t.Fatalf("err = %q, want it to contain %q", err, fragment)
	}
}

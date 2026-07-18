package service_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestCreateRegistration_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	expiresIn := time.Hour
	inv, err := env.Service.CreateRegistration("testnet", "new-peer", service.RegistrationOptions{IP: ip, ExpiresIn: &expiresIn})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	if inv.Network.Name != "testnet" {
		t.Errorf("network_name = %q, want testnet", inv.Network.Name)
	}
	if inv.Peer.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if inv.Peer.Route == "" {
		t.Error("route should not be empty")
	}
	if inv.Network.PublicKey == "" {
		t.Error("server public_key should not be empty")
	}
	if inv.Network.Endpoint != "192.168.1.1:51821" {
		t.Errorf("endpoint = %q, want 192.168.1.1:51821", inv.Network.Endpoint)
	}
	if _, err := env.Database.GetCidr("testnet", "new-peer"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("registration CIDR: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Service.GetPeer("testnet", "new-peer"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("registration peer: err = %v, want ErrNotFound", err)
	}
}

func TestCreateRegistration_DefaultExpiration(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.6")
	_, err := env.Service.CreateRegistration("testnet", "default-exp", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}

	expectedExpiry := testutil.FixedTime.Add(24 * time.Hour)
	if !regs[0].ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("expires_at = %v, want %v", regs[0].ExpiresAt, expectedExpiry)
	}
}

func TestCreateRegistration_MissingIP(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateRegistration("testnet", "no-ip", service.RegistrationOptions{})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRegistration_ReconcilesRunningInviteDeviceWithHostRoute(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.EnableNetwork("testnet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	ip := net.ParseIP("10.0.0.5")
	inv, err := env.Service.CreateRegistration("testnet", "live-reg", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

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

func TestCreateRegistration_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	_, err := env.Service.CreateRegistration("testnet", "", service.RegistrationOptions{IP: ip})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateRegistration_NonexistentNetwork(t *testing.T) {
	env := testutil.SetupService(t)

	ip := net.ParseIP("10.0.0.5")
	_, err := env.Service.CreateRegistration("nonexistent", "peer", service.RegistrationOptions{IP: ip})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateRegistration_DuplicateIP(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{IP: ip}); err != nil {
		t.Fatalf("create first registration: %v", err)
	}
	if _, err := env.Service.CreateRegistration("testnet", "bob", service.RegistrationOptions{IP: ip}); !errors.Is(err, service.ErrConflict) {
		t.Errorf("duplicate IP: err = %v, want ErrConflict", err)
	}
}

func TestRedeemRegistration_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	_, err := env.Service.CreateRegistration("testnet", "redeemer", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	result, err := env.Service.RedeemRegistration("testnet", tempKey, "perm-key-1")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if result.Network.Name != "testnet" {
		t.Errorf("network_name = %q, want testnet", result.Network.Name)
	}
	if result.Peer.Route != "10.0.0.5/32" {
		t.Errorf("route = %q, want 10.0.0.5/32", result.Peer.Route)
	}
	if result.Network.PublicKey == "" {
		t.Error("server public_key should not be empty")
	}

	peer, err := env.Service.GetPeer("testnet", "redeemer")
	if err != nil {
		t.Fatalf("get redeemed peer: %v", err)
	}
	if peer.PublicKey != "perm-key-1" {
		t.Errorf("public_key = %q, want perm-key-1", peer.PublicKey)
	}
	if peer.Route != "10.0.0.5/32" {
		t.Errorf("route = %q, want 10.0.0.5/32", peer.Route)
	}
	cidr, err := env.Database.GetCidr("testnet", "redeemer")
	if err != nil {
		t.Fatalf("get redeemed CIDR: %v", err)
	}
	if !cidr.Terminal || cidr.Cidr != "10.0.0.5/32" {
		t.Errorf("redeemed CIDR = %+v, want terminal 10.0.0.5/32", cidr)
	}
}

func TestRedeemRegistration_Idempotent_SameKey(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.6")
	_, err := env.Service.CreateRegistration("testnet", "idempotent", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	result1, err := env.Service.RedeemRegistration("testnet", tempKey, "perm-key-2")
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	result2, err := env.Service.RedeemRegistration("testnet", tempKey, "perm-key-2")
	if err != nil {
		t.Fatalf("second redeem: %v", err)
	}

	if result1.Peer.Route != result2.Peer.Route {
		t.Errorf("results differ: %q vs %q", result1.Peer.Route, result2.Peer.Route)
	}
}

func TestRedeemRegistration_UnknownKey(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	_, err := env.Service.CreateRegistration("testnet", "peer", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	_, err = env.Service.RedeemRegistration("testnet", "unknown-temp-key", "perm-key")
	if err == nil {
		t.Fatal("expected error for unknown temp key")
	}
}

func TestRedeemRegistration_MultipleRegistrations(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ipA := net.ParseIP("10.0.0.10")
	_, err := env.Service.CreateRegistration("testnet", "peer-a", service.RegistrationOptions{IP: ipA})
	if err != nil {
		t.Fatalf("create registration a: %v", err)
	}
	tempKey1 := lastTempKey(t, env.Service, "testnet")

	ipB := net.ParseIP("10.0.0.11")
	_, err = env.Service.CreateRegistration("testnet", "peer-b", service.RegistrationOptions{IP: ipB})
	if err != nil {
		t.Fatalf("create registration b: %v", err)
	}
	tempKey2 := lastTempKey(t, env.Service, "testnet")

	keyA := mustGenKey(t)
	keyB := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey1, keyA)
	if err != nil {
		t.Fatalf("redeem a: %v", err)
	}

	_, err = env.Service.RedeemRegistration("testnet", tempKey2, keyB)
	if err != nil {
		t.Fatalf("redeem b: %v", err)
	}

	peers, err := env.Service.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers (cord-server + peer-a + peer-b), got %d", len(peers))
	}
}

func TestRedeemRegistration_ReconcilesRunningDevices(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.EnableNetwork("testnet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}
	ip := net.ParseIP("10.0.0.5")
	_, err := env.Service.CreateRegistration("testnet", "live-redeem", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	permKey := mustGenKey(t)
	_, err = env.Service.RedeemRegistration("testnet", tempKey, permKey)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if env.Backend.Device("testnet-i") == nil {
		t.Fatal("expected invite device")
	}
	if peers := env.Backend.LastAppliedOpsFor("testnet-i"); len(peers) != 1 {
		t.Fatalf("invite peers after redeem = %d, want 1 (temp peer still active)", len(peers))
	}

	if env.Backend.Device("testnet") == nil {
		t.Fatal("expected main device")
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

func TestListRegistrations_Mixed(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ipActive := net.ParseIP("10.0.0.20")
	_, err := env.Service.CreateRegistration("testnet", "active", service.RegistrationOptions{IP: ipActive})
	if err != nil {
		t.Fatalf("create active: %v", err)
	}

	ipRedeem := net.ParseIP("10.0.0.21")
	_, err = env.Service.CreateRegistration("testnet", "to-redeem", service.RegistrationOptions{IP: ipRedeem})
	if err != nil {
		t.Fatalf("create to-redeem: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "redeemed-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(regs) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(regs))
	}

	var used, active int
	for _, reg := range regs {
		if reg.RedeemedKey != "" {
			used++
		} else {
			active++
		}
	}
	if used != 1 {
		t.Errorf("expected 1 used (redeemed_key set), got %d", used)
	}
	if active != 1 {
		t.Errorf("expected 1 active, got %d", active)
	}
	for _, reg := range regs {
		if reg.RedeemedKey != "" && !reg.Redeemed {
			t.Errorf("registration %q: should be redeemed after RedeemRegistration", reg.Name)
		}
		if reg.RedeemedKey == "" && reg.Redeemed {
			t.Errorf("registration %q: should not be redeemed without redeemed_key", reg.Name)
		}
	}
}

func TestRevokeRegistration_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.30")
	_, err := env.Service.CreateRegistration("testnet", "revoke-me", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := env.Service.RevokeRegistration("testnet", "revoke-me"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(regs) != 0 {
		t.Errorf("expected 0 registrations after revoke, got %d", len(regs))
	}
	if _, err := env.Service.CreateRegistration("testnet", "revoke-me", service.RegistrationOptions{IP: ip}); err != nil {
		t.Fatalf("reuse revoked registration name and IP: %v", err)
	}
}

func TestRevokeRegistration_RemovesProvisionalPeer(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.31")
	if _, err := env.Service.CreateRegistration("testnet", "revoke-me", service.RegistrationOptions{IP: ip}); err != nil {
		t.Fatalf("create: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	if _, err := env.Service.RedeemRegistration("testnet", tempKey, mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.Service.RevokeRegistration("testnet", "revoke-me"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := env.Service.GetPeer("testnet", "revoke-me"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("peer after revoke: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Database.GetCidr("testnet", "revoke-me"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("CIDR after revoke: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Service.CreateRegistration("testnet", "revoke-me", service.RegistrationOptions{IP: ip}); err != nil {
		t.Fatalf("reuse revoked provisional peer name and IP: %v", err)
	}
}

func TestRevokeRegistration_RejectsConfirmed(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.CreateRegistration("testnet", "confirmed", service.RegistrationOptions{IP: net.ParseIP("10.0.0.32")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "confirmed"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := env.Service.RevokeRegistration("testnet", "confirmed"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("revoke confirmed registration: err = %v, want ErrConflict", err)
	}
	reg, err := env.Database.GetRegistration("testnet", "confirmed")
	if err != nil {
		t.Fatalf("confirmed registration should remain: %v", err)
	}
	if !reg.Confirmed {
		t.Fatal("registration should remain confirmed")
	}
}

func TestRevokeRegistration_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.RevokeRegistration("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistration_Persistence(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	expiresIn := 2 * time.Hour
	_, err := env.Service.CreateRegistration("testnet", "persist-test", service.RegistrationOptions{IP: ip, Admin: true, ExpiresIn: &expiresIn})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}

	reg := regs[0]
	if reg.Name != "persist-test" {
		t.Errorf("name = %q, want persist-test", reg.Name)
	}
	if reg.InvitePublicKey == "" {
		t.Error("temp_pub_key should not be empty")
	}
	if !reg.Admin {
		t.Error("admin should be true")
	}
	if reg.Redeemed {
		t.Error("should not be redeemed")
	}
	if reg.RedeemedKey != "" {
		t.Errorf("redeemed_key = %q, want empty", reg.RedeemedKey)
	}
	if reg.MainRoute == "" || reg.MainRoute != "10.0.0.5/32" {
		t.Errorf("final_route = %q, want 10.0.0.5/32", reg.MainRoute)
	}
	if reg.InviteRoute == "" {
		t.Error("temp_ip should not be nil")
	}
	if reg.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
	if reg.ExpiresAt.IsZero() {
		t.Error("expires_at should not be zero")
	}
}

func TestRegistrationGroups_TransferOnConfirmation(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	for _, name := range []string{"engineering", "operations"} {
		if _, err := env.Service.CreateGroup("testnet", name); err != nil {
			t.Fatalf("create group %q: %v", name, err)
		}
	}
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{IP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "alice", "engineering"); err != nil {
		t.Fatalf("assign registration group: %v", err)
	}
	assertRegistrationGroups(t, env.Service, "alice", []string{"engineering"})
	if _, err := env.Service.GetCidr("testnet", "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("CIDR before redemption: err = %v, want ErrNotFound", err)
	}

	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "alice", "operations"); err != nil {
		t.Fatalf("assign registration group after redemption: %v", err)
	}
	if err := env.Service.AssignCidrGroup("testnet", "alice", "operations"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("assign group directly to provisional CIDR: err = %v, want ErrConflict", err)
	}
	assertRegistrationGroups(t, env.Service, "alice", []string{"engineering", "operations"})
	assertTransferredCidrGroups(t, env.Service, "alice", nil)

	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm peer: %v", err)
	}
	assertRegistrationGroups(t, env.Service, "alice", nil)
	assertTransferredCidrGroups(t, env.Service, "alice", []string{"engineering", "operations"})
	if err := env.Service.AssignRegistrationGroup("testnet", "alice", "engineering"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("modify confirmed registration: err = %v, want ErrConflict", err)
	}
}

func TestRegistrationGroups_Remove(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{IP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "alice", "engineering"); err != nil {
		t.Fatalf("assign registration group: %v", err)
	}
	if err := env.Service.RemoveRegistrationGroup("testnet", "alice", "engineering"); err != nil {
		t.Fatalf("remove registration group: %v", err)
	}
	assertRegistrationGroups(t, env.Service, "alice", nil)
}

func TestPruneExpiredRegistrations_RemovesExpiredProvisionalPeer(t *testing.T) {
	clock := &mutableClock{t: testutil.FixedTime}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.EnableNetwork("testnet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	shortIP := net.ParseIP("10.0.0.7")
	shortExpiry := time.Hour
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"short-lived",
		service.RegistrationOptions{IP: shortIP, ExpiresIn: &shortExpiry},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	shortKey := mustGenKey(t)
	if _, err := env.Service.RedeemRegistration(
		"testnet",
		lastTempKey(t, env.Service, "testnet"),
		shortKey,
	); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	clock.t = clock.t.Add(2 * time.Hour)
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"reconcile-trigger",
		service.RegistrationOptions{IP: net.ParseIP("10.0.0.250")},
	); err != nil {
		t.Fatalf("trigger reconcile: %v", err)
	}

	if _, err := env.Service.GetPeer("testnet", "short-lived"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("peer after expiry: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Database.GetRegistration("testnet", "short-lived"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("registration after expiry: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Database.GetCidr("testnet", "short-lived"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("CIDR after expiry: err = %v, want ErrNotFound", err)
	}
	if hasPeerOp(env.Backend.LastAppliedOpsFor("testnet"), shortKey) {
		t.Error("main device should not have expired provisional peer")
	}
	if _, err := env.Service.CreateRegistration("testnet", "short-lived", service.RegistrationOptions{IP: shortIP}); err != nil {
		t.Fatalf("reuse expired registration name and IP: %v", err)
	}
}

func TestPruneExpiredRegistrations_RetainsActiveProvisionalPeer(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	stillExpiry := 24 * time.Hour
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"still-active",
		service.RegistrationOptions{IP: net.ParseIP("10.0.0.8"), ExpiresIn: &stillExpiry},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration(
		"testnet",
		lastTempKey(t, env.Service, "testnet"),
		mustGenKey(t),
	); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"reconcile-trigger",
		service.RegistrationOptions{IP: net.ParseIP("10.0.0.250")},
	); err != nil {
		t.Fatalf("trigger reconcile: %v", err)
	}

	peer, err := env.Service.GetPeer("testnet", "still-active")
	if err != nil {
		t.Fatalf("get peer after reconcile: %v", err)
	}
	if peer.Confirmed {
		t.Error("peer should still be provisional")
	}
}

func TestPruneExpiredRegistrations_RetainsConfirmedRegistrationAsAudit(t *testing.T) {
	clock := &mutableClock{t: testutil.FixedTime}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)

	auditedExpiry := time.Hour
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"audited",
		service.RegistrationOptions{IP: net.ParseIP("10.0.0.10"), ExpiresIn: &auditedExpiry},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration(
		"testnet",
		lastTempKey(t, env.Service, "testnet"),
		mustGenKey(t),
	); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "audited"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	clock.t = clock.t.Add(2 * time.Hour)
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"reconcile-trigger",
		service.RegistrationOptions{IP: net.ParseIP("10.0.0.250")},
	); err != nil {
		t.Fatalf("trigger reconcile: %v", err)
	}
	reg, err := env.Database.GetRegistration("testnet", "audited")
	if err != nil {
		t.Fatalf("get registration after expiry: %v", err)
	}
	if !reg.Confirmed {
		t.Error("registration should remain confirmed")
	}
}

type mutableClock struct {
	t time.Time
}

func (m *mutableClock) now() time.Time { return m.t }

func assertRegistrationGroups(t *testing.T, svc *service.Service, registration string, want []string) {
	t.Helper()
	groups, err := svc.ListRegistrationGroups("testnet", registration)
	if err != nil {
		t.Fatalf("list registration groups: %v", err)
	}
	if len(groups) != len(want) {
		t.Fatalf("registration groups = %+v, want %v", groups, want)
	}
	for i, group := range groups {
		if group.Name != want[i] {
			t.Fatalf("registration group %d = %q, want %q", i, group.Name, want[i])
		}
	}
}

func assertTransferredCidrGroups(t *testing.T, svc *service.Service, cidr string, want []string) {
	t.Helper()
	groups, err := svc.ListCidrGroups("testnet", cidr)
	if err != nil {
		t.Fatalf("list CIDR groups: %v", err)
	}
	if len(groups) != len(want) {
		t.Fatalf("CIDR %q groups = %+v, want %v", cidr, groups, want)
	}
	for i, group := range groups {
		if group.Name != want[i] {
			t.Fatalf("CIDR %q group %d = %q, want %q", cidr, i, group.Name, want[i])
		}
	}
}

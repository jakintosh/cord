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

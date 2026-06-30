package database_test

import (
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForRegistration(t *testing.T, db *database.DB) {
	t.Helper()
	now := time.Now()
	if err := db.BootstrapNetwork(&service.Network{
		Name:                "regnet",
		PrivateKey:          "priv",
		PublicKey:           "pub",
		MainCidr:            "10.0.0.0/16",
		InviteCidr:          "10.1.0.0/24",
		ExternalIP:          "1.1.1.1",
		MainWireguardPort:   51820,
		InviteWireguardPort: 51821,
		MainApiPort:         80,
		InviteApiPort:       80,
		CreatedAt:           now,
	}, &service.Cidr{Name: "regnet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

func TestInsertAndGetRegistration(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	expires := now.Add(24 * time.Hour)
	reg := &service.Registration{
		Name:            "reg-1",
		InvitePublicKey: "temp-key-1",
		InviteIP:        net.IPv4(10, 1, 0, 1),
		MainIP:          net.IPv4(10, 0, 5, 1),
		Admin:           true,
		Redeemed:        false,
		RedeemedKey:     "",
		ExpiresAt:       expires,
		CreatedAt:       now,
	}

	if err := db.InsertRegistration("regnet", reg); err != nil {
		t.Fatalf("insert registration: %v", err)
	}

	got, err := db.GetRegistration("regnet", "reg-1")
	if err != nil {
		t.Fatalf("get registration: %v", err)
	}

	if got.Name != reg.Name {
		t.Errorf("name = %q, want %q", got.Name, reg.Name)
	}
	if got.InvitePublicKey != reg.InvitePublicKey {
		t.Errorf("temp_pub_key = %q, want %q", got.InvitePublicKey, reg.InvitePublicKey)
	}
	if !got.InviteIP.Equal(reg.InviteIP) {
		t.Errorf("temp_ip = %v, want %v", got.InviteIP, reg.InviteIP)
	}
	if !got.MainIP.Equal(reg.MainIP) {
		t.Errorf("final_ip = %v, want %v", got.MainIP, reg.MainIP)
	}
	if got.Admin != reg.Admin {
		t.Errorf("admin = %v, want %v", got.Admin, reg.Admin)
	}
	if got.Redeemed != reg.Redeemed {
		t.Errorf("redeemed = %v, want %v", got.Redeemed, reg.Redeemed)
	}
	if got.ExpiresAt.Unix() != expires.Unix() {
		t.Errorf("expires_at = %v, want %v", got.ExpiresAt, expires)
	}
	if got.CreatedAt.Unix() != now.Unix() {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, now)
	}
}

func TestGetRegistration_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	_, err := db.GetRegistration("regnet", "nobody")
	if err == nil {
		t.Fatal("expected error for nonexistent registration")
	}
}

func TestGetRegistrationByIP(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	expires := now.Add(24 * time.Hour)
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name:            "ip-reg",
		InvitePublicKey: "ip-key",
		InviteIP:        net.IPv4(10, 1, 0, 10),
		MainIP:          net.IPv4(10, 0, 5, 10),
		Admin:           false,
		ExpiresAt:       expires,
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("insert registration: %v", err)
	}

	got, err := db.GetRegistrationByIP("regnet", net.IPv4(10, 1, 0, 10), now)
	if err != nil {
		t.Fatalf("get registration by IP: %v", err)
	}
	if got.Name != "ip-reg" {
		t.Errorf("name = %q, want ip-reg", got.Name)
	}
}

func TestGetRegistrationByIP_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)
	now := time.Now()

	_, err := db.GetRegistrationByIP("regnet", net.IPv4(10, 1, 0, 99), now)
	if err == nil {
		t.Fatal("expected error for unknown IP")
	}
}

func TestListRegistrations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	expires := now.Add(24 * time.Hour)
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "zzz", InvitePublicKey: "z-key", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), ExpiresAt: expires, CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert zzz: %v", err)
	}
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "aaa", InvitePublicKey: "a-key", InviteIP: net.IPv4(10, 1, 0, 2),
		MainIP: net.IPv4(10, 0, 5, 2), ExpiresAt: expires, CreatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("insert aaa: %v", err)
	}

	regs, err := db.ListRegistrations("regnet")
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(regs))
	}
	// Sorted by created_at DESC, so aaa (newer) first, then zzz
	if regs[0].Name != "aaa" || regs[1].Name != "zzz" {
		t.Errorf("unexpected order: %v, %v", regs[0].Name, regs[1].Name)
	}
}

func TestListActiveRegistrations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "active", InvitePublicKey: "act-key", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert active: %v", err)
	}
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "expired", InvitePublicKey: "exp-key", InviteIP: net.IPv4(10, 1, 0, 2),
		MainIP: net.IPv4(10, 0, 5, 2), ExpiresAt: now.Add(-1 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert expired: %v", err)
	}

	active, err := db.ListActiveRegistrations("regnet", now)
	if err != nil {
		t.Fatalf("list active registrations: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active registration, got %d", len(active))
	}
	if active[0].Name != "active" {
		t.Errorf("expected active, got %s", active[0].Name)
	}
}

func TestListActiveRegistrations_IncludesRedeemed(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "redeemed-one", InvitePublicKey: "red-key", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), Redeemed: true, RedeemedKey: "perm-key",
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert redeemed: %v", err)
	}

	active, err := db.ListActiveRegistrations("regnet", now)
	if err != nil {
		t.Fatalf("list active registrations: %v", err)
	}
	// Redeemed registrations remain active on the invite device until
	// ConfirmPeer marks them confirmed, so the temp peer can still
	// receive retries if the client's redeem response was lost.
	if len(active) != 1 {
		t.Fatalf("expected 1 active registration (redeemed still active on invite device), got %d", len(active))
	}
}

func TestDeleteRegistration(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "delme", InvitePublicKey: "del-key", InviteIP: net.IPv4(10, 1, 0, 99),
		MainIP: net.IPv4(10, 0, 5, 99), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert registration: %v", err)
	}

	if err := db.DeleteRegistration("regnet", "delme"); err != nil {
		t.Fatalf("delete registration: %v", err)
	}

	_, err := db.GetRegistration("regnet", "delme")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteRegistration_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	err := db.DeleteRegistration("regnet", "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent registration")
	}
}

func TestDeleteExpiredRegistrations(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "old", InvitePublicKey: "old-key", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(-2 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "new", InvitePublicKey: "new-key", InviteIP: net.IPv4(10, 1, 0, 2),
		MainIP: net.IPv4(10, 0, 5, 2), ExpiresAt: now.Add(2 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert new: %v", err)
	}

	if err := db.DeleteExpiredRegistrations("regnet", now); err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	regs, err := db.ListRegistrations("regnet")
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].Name != "new" {
		t.Errorf("expected 'new', got %q", regs[0].Name)
	}
}

func TestUpdateRegistrationRedemption(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "redeem-me", InvitePublicKey: "temp-reg-key", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), Admin: true, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert registration: %v", err)
	}

	if err := db.RedeemRegistration("regnet", "temp-reg-key", "perm-peer-key", now); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}

	reg, err := db.GetRegistration("regnet", "redeem-me")
	if err != nil {
		t.Fatalf("get registration: %v", err)
	}
	if !reg.Redeemed {
		t.Error("registration should be redeemed")
	}
	if reg.RedeemedKey != "perm-peer-key" {
		t.Errorf("redeemed_key = %q, want perm-peer-key", reg.RedeemedKey)
	}

	peer, err := db.GetPeer("regnet", "redeem-me")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.PublicKey != "perm-peer-key" {
		t.Errorf("peer public_key = %q, want perm-peer-key", peer.PublicKey)
	}
	if peer.Admin != true {
		t.Error("peer should be admin")
	}
	if peer.Confirmed != false {
		t.Error("peer should not be confirmed yet")
	}
	if !peer.Enabled {
		t.Error("peer should be enabled immediately for /confirm")
	}
}

func TestUpdateRegistrationRedemption_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)
	now := time.Now()

	err := db.RedeemRegistration("regnet", "unknown-key", "perm-key", now)
	if err == nil {
		t.Fatal("expected error for unknown registration key")
	}
}

func TestUpdateRegistrationRedemption_DoubleRedeem(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	if err := db.InsertRegistration("regnet", &service.Registration{
		Name: "redeem-twice", InvitePublicKey: "twice-key", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert registration: %v", err)
	}

	if err := db.RedeemRegistration("regnet", "twice-key", "perm-key-1", now); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	err := db.RedeemRegistration("regnet", "twice-key", "perm-key-2", now)
	if err == nil {
		t.Fatal("expected error for double redeem")
	}
}

func TestInsertRegistration_DuplicateName(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForRegistration(t, db)

	now := time.Now()
	reg := &service.Registration{
		Name: "dup", InvitePublicKey: "key-a", InviteIP: net.IPv4(10, 1, 0, 1),
		MainIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}
	if err := db.InsertRegistration("regnet", reg); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	reg.InvitePublicKey = "key-b"
	reg.InviteIP = net.IPv4(10, 1, 0, 2)
	reg.MainIP = net.IPv4(10, 0, 5, 2)
	err := db.InsertRegistration("regnet", reg)
	if err == nil {
		t.Fatal("expected error for duplicate registration name")
	}
}

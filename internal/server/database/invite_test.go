package database_test

import (
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedNetworkForInvite(t *testing.T, db *database.DB) {
	t.Helper()
	now := time.Now()
	if err := db.BootstrapNetwork(&service.Network{
		Name:             "invitenet",
		PrivateKey:       "priv",
		PublicKey:        "pub",
		MainCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "1.1.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
		CreatedAt:        now,
	}, &service.Cidr{Name: "invitenet", Cidr: "10.0.0.0/16", Length: 16, Prefix: 32}, &service.Peer{Name: "cord-server", Cidr: "10.0.0.1/32", PublicKey: "pub", Admin: true, Enabled: true, Confirmed: true}); err != nil {
		t.Fatalf("seed network: %v", err)
	}
}

func TestInsertAndGetInvite(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	expires := now.Add(24 * time.Hour)
	inv := &service.Invite{
		Name:        "invite-1",
		TempPubKey:  "temp-key-1",
		TempIP:      net.IPv4(10, 1, 0, 1),
		FinalIP:     net.IPv4(10, 0, 5, 1),
		Admin:       true,
		Redeemed:    false,
		RedeemedKey: "",
		ExpiresAt:   expires,
		CreatedAt:   now,
	}

	if err := db.InsertInvite("invitenet", inv); err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	got, err := db.GetInvite("invitenet", "invite-1")
	if err != nil {
		t.Fatalf("get invite: %v", err)
	}

	if got.Name != inv.Name {
		t.Errorf("name = %q, want %q", got.Name, inv.Name)
	}
	if got.TempPubKey != inv.TempPubKey {
		t.Errorf("temp_pub_key = %q, want %q", got.TempPubKey, inv.TempPubKey)
	}
	if !got.TempIP.Equal(inv.TempIP) {
		t.Errorf("temp_ip = %v, want %v", got.TempIP, inv.TempIP)
	}
	if !got.FinalIP.Equal(inv.FinalIP) {
		t.Errorf("final_ip = %v, want %v", got.FinalIP, inv.FinalIP)
	}
	if got.Admin != inv.Admin {
		t.Errorf("admin = %v, want %v", got.Admin, inv.Admin)
	}
	if got.Redeemed != inv.Redeemed {
		t.Errorf("redeemed = %v, want %v", got.Redeemed, inv.Redeemed)
	}
	if got.ExpiresAt.Unix() != expires.Unix() {
		t.Errorf("expires_at = %v, want %v", got.ExpiresAt, expires)
	}
	if got.CreatedAt.Unix() != now.Unix() {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, now)
	}
}

func TestGetInvite_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	_, err := db.GetInvite("invitenet", "nobody")
	if err == nil {
		t.Fatal("expected error for nonexistent invite")
	}
}

func TestGetInviteByIP(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	expires := now.Add(24 * time.Hour)
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name:       "ip-invite",
		TempPubKey: "ip-key",
		TempIP:     net.IPv4(10, 1, 0, 10),
		FinalIP:    net.IPv4(10, 0, 5, 10),
		Admin:      false,
		ExpiresAt:  expires,
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	got, err := db.GetInviteByIP("invitenet", net.IPv4(10, 1, 0, 10), now)
	if err != nil {
		t.Fatalf("get invite by IP: %v", err)
	}
	if got.Name != "ip-invite" {
		t.Errorf("name = %q, want ip-invite", got.Name)
	}
}

func TestGetInviteByIP_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)
	now := time.Now()

	_, err := db.GetInviteByIP("invitenet", net.IPv4(10, 1, 0, 99), now)
	if err == nil {
		t.Fatal("expected error for unknown IP")
	}
}

func TestListInvites(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	expires := now.Add(24 * time.Hour)
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "zzz", TempPubKey: "z-key", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), ExpiresAt: expires, CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert zzz: %v", err)
	}
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "aaa", TempPubKey: "a-key", TempIP: net.IPv4(10, 1, 0, 2),
		FinalIP: net.IPv4(10, 0, 5, 2), ExpiresAt: expires, CreatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("insert aaa: %v", err)
	}

	invites, err := db.ListInvites("invitenet")
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invites, got %d", len(invites))
	}
	// Sorted by created_at DESC, so aaa (newer) first, then zzz
	if invites[0].Name != "aaa" || invites[1].Name != "zzz" {
		t.Errorf("unexpected order: %v, %v", invites[0].Name, invites[1].Name)
	}
}

func TestListActiveInvites(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "active", TempPubKey: "act-key", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert active: %v", err)
	}
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "expired", TempPubKey: "exp-key", TempIP: net.IPv4(10, 1, 0, 2),
		FinalIP: net.IPv4(10, 0, 5, 2), ExpiresAt: now.Add(-1 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert expired: %v", err)
	}

	active, err := db.ListActiveInvites("invitenet", now)
	if err != nil {
		t.Fatalf("list active invites: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active invite, got %d", len(active))
	}
	if active[0].Name != "active" {
		t.Errorf("expected active, got %s", active[0].Name)
	}
}

func TestListActiveInvites_IncludesRedeemed(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "redeemed-one", TempPubKey: "red-key", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), Redeemed: true, RedeemedKey: "perm-key",
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert redeemed: %v", err)
	}

	active, err := db.ListActiveInvites("invitenet", now)
	if err != nil {
		t.Fatalf("list active invites: %v", err)
	}
	// Redeemed invites remain active on the invite device until
	// ConfirmPeer marks them confirmed, so the temp peer can still
	// receive retries if the client's redeem response was lost.
	if len(active) != 1 {
		t.Fatalf("expected 1 active invite (redeemed still active on invite device), got %d", len(active))
	}
}

func TestDeleteInvite(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "delme", TempPubKey: "del-key", TempIP: net.IPv4(10, 1, 0, 99),
		FinalIP: net.IPv4(10, 0, 5, 99), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	if err := db.DeleteInvite("invitenet", "delme"); err != nil {
		t.Fatalf("delete invite: %v", err)
	}

	_, err := db.GetInvite("invitenet", "delme")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteInvite_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	err := db.DeleteInvite("invitenet", "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent invite")
	}
}

func TestDeleteExpiredInvites(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "old", TempPubKey: "old-key", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(-2 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "new", TempPubKey: "new-key", TempIP: net.IPv4(10, 1, 0, 2),
		FinalIP: net.IPv4(10, 0, 5, 2), ExpiresAt: now.Add(2 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert new: %v", err)
	}

	if err := db.DeleteExpiredInvites("invitenet", now); err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	invites, err := db.ListInvites("invitenet")
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(invites))
	}
	if invites[0].Name != "new" {
		t.Errorf("expected 'new', got %q", invites[0].Name)
	}
}

func TestUpdateInviteRedemption(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "redeem-me", TempPubKey: "temp-invite-key", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), Admin: true, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	if err := db.RedeemInvite("invitenet", "temp-invite-key", "perm-peer-key", now); err != nil {
		t.Fatalf("redeem invite: %v", err)
	}

	inv, err := db.GetInvite("invitenet", "redeem-me")
	if err != nil {
		t.Fatalf("get invite: %v", err)
	}
	if !inv.Redeemed {
		t.Error("invite should be redeemed")
	}
	if inv.RedeemedKey != "perm-peer-key" {
		t.Errorf("redeemed_key = %q, want perm-peer-key", inv.RedeemedKey)
	}

	peer, err := db.GetPeer("invitenet", "redeem-me")
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

func TestUpdateInviteRedemption_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)
	now := time.Now()

	err := db.RedeemInvite("invitenet", "unknown-key", "perm-key", now)
	if err == nil {
		t.Fatal("expected error for unknown invite key")
	}
}

func TestUpdateInviteRedemption_DoubleRedeem(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	if err := db.InsertInvite("invitenet", &service.Invite{
		Name: "redeem-twice", TempPubKey: "twice-key", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	if err := db.RedeemInvite("invitenet", "twice-key", "perm-key-1", now); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	err := db.RedeemInvite("invitenet", "twice-key", "perm-key-2", now)
	if err == nil {
		t.Fatal("expected error for double redeem")
	}
}

func TestInsertInvite_DuplicateName(t *testing.T) {
	db := testutil.SetupDB(t)
	seedNetworkForInvite(t, db)

	now := time.Now()
	inv := &service.Invite{
		Name: "dup", TempPubKey: "key-a", TempIP: net.IPv4(10, 1, 0, 1),
		FinalIP: net.IPv4(10, 0, 5, 1), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	}
	if err := db.InsertInvite("invitenet", inv); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	inv.TempPubKey = "key-b"
	inv.TempIP = net.IPv4(10, 1, 0, 2)
	inv.FinalIP = net.IPv4(10, 0, 5, 2)
	err := db.InsertInvite("invitenet", inv)
	if err == nil {
		t.Fatal("expected error for duplicate invite name")
	}
}

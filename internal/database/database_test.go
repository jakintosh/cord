package database_test

import (
	"net"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/database"
)

// TestInviteDesc describes an invite for testing
type TestInviteDesc struct {
	Name       string
	PubKey     string
	TempIP     net.IP
	FinalIP    net.IP
	Admin      bool
	Expiration int64
}

// Common test invites
var (
	TestUser1 = TestInviteDesc{
		Name:       "test-user",
		PubKey:     "abc123key",
		TempIP:     net.IPv4(10, 0, 64, 1),
		FinalIP:    net.IPv4(10, 0, 128, 1),
		Admin:      false,
		Expiration: time.Now().Add(24 * time.Hour).Unix(),
	}

	TestUser2 = TestInviteDesc{
		Name:       "test-user-2",
		PubKey:     "def456key",
		TempIP:     net.IPv4(10, 0, 64, 2),
		FinalIP:    net.IPv4(10, 0, 128, 2),
		Admin:      false,
		Expiration: time.Now().Add(48 * time.Hour).Unix(),
	}

	TestAdmin = TestInviteDesc{
		Name:       "admin-user",
		PubKey:     "xyz789key",
		TempIP:     net.IPv4(10, 0, 64, 10),
		FinalIP:    net.IPv4(10, 0, 128, 10),
		Admin:      true,
		Expiration: time.Now().Add(48 * time.Hour).Unix(),
	}

	ExpiredInvite = TestInviteDesc{
		Name:       "expired-user",
		PubKey:     "expired123key",
		TempIP:     net.IPv4(10, 0, 64, 99),
		FinalIP:    net.IPv4(10, 0, 128, 99),
		Admin:      false,
		Expiration: time.Now().Add(-24 * time.Hour).Unix(), // expired
	}
)

// setupTestDB creates a new in-memory SQLite database for testing
func setupTestDB(t *testing.T) *database.SQLiteStore {
	t.Helper()
	store, err := database.Init("test-network", ":memory:", false)
	if err != nil {
		t.Fatalf("failed to init test database: %v", err)
	}
	return store
}

// createInvite creates an invite from a TestInviteDesc
func createInvite(
	t *testing.T,
	store *database.SQLiteStore,
	desc TestInviteDesc,
) error {
	t.Helper()
	return store.InviteCreate(
		desc.Name,
		desc.PubKey,
		desc.TempIP,
		desc.FinalIP,
		desc.Admin,
		desc.Expiration,
	)
}

// createInvites creates multiple invites from TestInviteDesc slice
func createInvites(
	t *testing.T,
	store *database.SQLiteStore,
	descs []TestInviteDesc,
) error {
	t.Helper()
	for _, desc := range descs {
		if err := createInvite(t, store, desc); err != nil {
			return err
		}
	}
	return nil
}

// assertInviteExists verifies that an invite exists and matches expected values
func assertInviteExists(
	t *testing.T,
	store *database.SQLiteStore,
	desc TestInviteDesc,
) {
	t.Helper()
	invite, err := store.InviteGet(desc.Name)
	if err != nil {
		t.Fatalf("expected invite %s to exist, but got error: %v", desc.Name, err)
	}

	if invite.Name != desc.Name {
		t.Errorf("invite name = %v, want %v", invite.Name, desc.Name)
	}
	if invite.PublicKey != desc.PubKey {
		t.Errorf("invite public key = %v, want %v", invite.PublicKey, desc.PubKey)
	}
	if invite.Admin != desc.Admin {
		t.Errorf("invite admin = %v, want %v", invite.Admin, desc.Admin)
	}
	if invite.Expiration.Unix() != desc.Expiration {
		t.Errorf("invite expiration = %v, want %v", invite.Expiration.Unix(), desc.Expiration)
	}
}

// assertInviteNotExists verifies that an invite does not exist
func assertInviteNotExists(
	t *testing.T,
	store *database.SQLiteStore,
	name string,
) {
	t.Helper()
	_, err := store.InviteGet(name)
	if err == nil {
		t.Errorf("expected invite %s to not exist, but it was found", name)
	}
}

// assertInviteRedeemed verifies that an invite is marked as redeemed
func assertInviteRedeemed(
	t *testing.T,
	store *database.SQLiteStore,
	name string,
) {
	t.Helper()
	invite, err := store.InviteGet(name)
	if err != nil {
		t.Fatalf("failed to get invite %s: %v", name, err)
	}
	if !invite.Redeemed {
		t.Errorf("invite %s should be marked as redeemed", name)
	}
}

// assertInviteNotRedeemed verifies that an invite is not marked as redeemed
func assertInviteNotRedeemed(
	t *testing.T,
	store *database.SQLiteStore,
	name string,
) {
	t.Helper()
	invite, err := store.InviteGet(name)
	if err != nil {
		t.Fatalf("failed to get invite %s: %v", name, err)
	}
	if invite.Redeemed {
		t.Errorf("invite %s should not be marked as redeemed", name)
	}
}

// assertInviteCount verifies the total number of invites
func assertInviteCount(
	t *testing.T,
	store *database.SQLiteStore,
	expectedCount int,
) {
	t.Helper()
	invites, err := store.InviteList()
	if err != nil {
		t.Fatalf("failed to list invites: %v", err)
	}
	if len(invites) != expectedCount {
		t.Errorf("expected %d invites, got %d", expectedCount, len(invites))
	}
}

// assertPeerExists verifies that a peer was created with expected values
func assertPeerExists(
	t *testing.T,
	store *database.SQLiteStore,
	name, pubKey string,
	admin bool,
) {
	t.Helper()
	peer, err := store.PeerGet(name)
	if err != nil {
		t.Fatalf("expected peer %s to exist, but got error: %v", name, err)
	}

	if peer.Name != name {
		t.Errorf("peer name = %v, want %v", peer.Name, name)
	}
	if peer.PublicKey != pubKey {
		t.Errorf("peer public key = %v, want %v", peer.PublicKey, pubKey)
	}
	if peer.Admin != admin {
		t.Errorf("peer admin = %v, want %v", peer.Admin, admin)
	}
	if !peer.Enabled {
		t.Error("redeemed peer should be enabled")
	}
	if !peer.Confirmed {
		t.Error("redeemed peer should be confirmed")
	}
}

// expectError is a helper to assert that an operation should fail
func expectError(
	t *testing.T,
	err error,
	operation string,
) {
	t.Helper()
	if err == nil {
		t.Errorf("%s should have failed, but succeeded", operation)
	}
}

// expectNoError is a helper to assert that an operation should succeed
func expectNoError(
	t *testing.T,
	err error,
	operation string,
) {
	t.Helper()
	if err != nil {
		t.Errorf("%s should have succeeded, but failed with: %v", operation, err)
	}
}

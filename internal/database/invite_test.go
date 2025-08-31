package database_test

import (
	"testing"
	"time"
)

// TestInviteCreateValid tests creating a valid invite
func TestInviteCreateValid(t *testing.T) {
	store := setupTestDB(t)

	// create valid test user invite
	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating valid invite")
	assertInviteExists(t, store, TestUser1)
}

// TestInviteCreateAdmin tests creating an admin invite
func TestInviteCreateAdmin(t *testing.T) {
	store := setupTestDB(t)

	// create valid admin user invite
	err := createInvite(t, store, TestAdmin)
	expectNoError(t, err, "creating admin invite")
	assertInviteExists(t, store, TestAdmin)
}

// TestInviteCreateDuplicateName tests that duplicate names are rejected
func TestInviteCreateDuplicateName(t *testing.T) {
	store := setupTestDB(t)

	// create valid invite
	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating valid invite")

	// create duplicate invite, but with uniqe key
	duplicateInvite := TestUser1
	duplicateInvite.PubKey = "different-key"
	err = createInvite(t, store, duplicateInvite)
	expectError(t, err, "creating invite with duplicate name")
}

// TestInviteCreateDuplicatePublicKey tests that duplicate public keys are rejected
func TestInviteCreateDuplicatePublicKey(t *testing.T) {
	store := setupTestDB(t)

	// create valid invite
	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating valid invite")

	// create duplicate invite, but with different name
	duplicateInvite := TestUser1
	duplicateInvite.Name = "different-user"
	err = createInvite(t, store, duplicateInvite)
	expectError(t, err, "creating invite with duplicate public key")
}

// TestInviteGetExisting tests retrieving an existing invite
func TestInviteGetExisting(t *testing.T) {
	store := setupTestDB(t)

	err := createInvite(t, store, TestAdmin)
	expectNoError(t, err, "creating valid invite")

	invite, err := store.InviteGet(TestAdmin.Name)

	expectNoError(t, err, "getting existing invite")

	if invite.Name != TestAdmin.Name {
		t.Errorf("invite name = %v, want %v", invite.Name, TestAdmin.Name)
	}
	if invite.PublicKey != TestAdmin.PubKey {
		t.Errorf("invite public key = %v, want %v", invite.PublicKey, TestAdmin.PubKey)
	}
	if invite.Admin != TestAdmin.Admin {
		t.Errorf("invite admin = %v, want %v", invite.Admin, TestAdmin.Admin)
	}
	if invite.Redeemed {
		t.Errorf("invite redeemed = %v, want false", invite.Redeemed)
	}
	if invite.Expiration.Unix() != TestAdmin.Expiration {
		t.Errorf("invite expiration = %v, want %v", invite.Expiration.Unix(), TestAdmin.Expiration)
	}

	// Verify CIDR strings are properly formatted
	if invite.InviteCidr == "" {
		t.Error("invite CIDR is empty")
	}
	if invite.NetworkCidr == "" {
		t.Error("network CIDR is empty")
	}
}

// TestInviteGetNonExistent tests retrieving a non-existent invite
func TestInviteGetNonExistent(t *testing.T) {
	store := setupTestDB(t)

	_, err := store.InviteGet("non-existent")

	expectError(t, err, "getting non-existent invite")
}

// TestInviteListMultiple tests listing multiple invites
func TestInviteListMultiple(t *testing.T) {
	store := setupTestDB(t)
	invites := []TestInviteDesc{TestUser1, TestAdmin, TestUser2}
	createInvites(t, store, invites)

	result, err := store.InviteList()

	expectNoError(t, err, "listing invites")

	if len(result) != len(invites) {
		t.Errorf("returned %d invites, want %d", len(result), len(invites))
	}

	// Check that results are ordered by expiration DESC (as specified in SQL)
	for i := 1; i < len(result); i++ {
		if result[i-1].Expiration.Before(result[i].Expiration) {
			t.Errorf("results not ordered by expiration DESC: %v < %v",
				result[i-1].Expiration, result[i].Expiration)
		}
	}

	// Verify each invite has required fields populated
	for _, invite := range result {
		if invite.Name == "" {
			t.Error("invite has empty name")
		}
		if invite.PublicKey == "" {
			t.Error("invite has empty public key")
		}
		if invite.InviteCidr == "" {
			t.Error("invite has empty invite CIDR")
		}
		if invite.NetworkCidr == "" {
			t.Error("invite has empty network CIDR")
		}
	}
}

// TestInviteListEmpty tests listing when no invites exist
func TestInviteListEmpty(t *testing.T) {
	store := setupTestDB(t)

	result, err := store.InviteList()

	expectNoError(t, err, "listing empty invites")
	assertInviteCount(t, store, 0)

	if len(result) != 0 {
		t.Errorf("empty db returned %d invites, want 0", len(result))
	}
}

// TestInviteRedeemValid tests redeeming a valid invite
func TestInviteRedeemValid(t *testing.T) {
	store := setupTestDB(t)
	createInvite(t, store, TestUser1)

	newPubKey := "new-peer-key"
	err := store.InviteRedeem(TestUser1.PubKey, newPubKey)

	expectNoError(t, err, "redeeming valid invite")

	// Verify invite is marked as redeemed
	assertInviteRedeemed(t, store, TestUser1.Name)

	// Verify peer was created
	assertPeerExists(t, store, TestUser1.Name, newPubKey, TestUser1.Admin)
}

// TestInviteRedeemAlreadyRedeemed tests redeeming an already redeemed invite
func TestInviteRedeemAlreadyRedeemed(t *testing.T) {
	store := setupTestDB(t)
	createInvite(t, store, TestUser1)

	// Redeem the invite first
	firstKey := "first-new-key"
	err := store.InviteRedeem(TestUser1.PubKey, firstKey)
	expectNoError(t, err, "initial redeem")

	// Try to redeem again
	secondKey := "second-new-key"
	err = store.InviteRedeem(TestUser1.PubKey, secondKey)

	expectError(t, err, "redeeming already redeemed invite")
}

// TestInviteRedeemNonExistent tests redeeming a non-existent invite
func TestInviteRedeemNonExistent(t *testing.T) {
	store := setupTestDB(t)

	err := store.InviteRedeem("non-existent-key", "some-new-key")

	expectError(t, err, "redeeming non-existent invite")
}

// TestInviteRedeemAdmin tests redeeming an invite with admin privileges
func TestInviteRedeemAdmin(t *testing.T) {
	store := setupTestDB(t)
	createInvite(t, store, TestAdmin)

	adminNewKey := "admin-new-key"
	err := store.InviteRedeem(TestAdmin.PubKey, adminNewKey)

	expectNoError(t, err, "redeeming admin invite")

	// Verify admin peer was created with admin privileges
	assertPeerExists(t, store, TestAdmin.Name, adminNewKey, true)
}

// TestInviteRedeemExpired tests behavior with expired invites (if applicable)
func TestInviteRedeemExpired(t *testing.T) {
	store := setupTestDB(t)

	// Create an invite that's already expired
	expiredInvite := TestInviteDesc{
		Name:       "expired-user",
		PubKey:     "expired-key",
		TempIP:     TestUser1.TempIP,
		FinalIP:    TestUser1.FinalIP,
		Admin:      false,
		Expiration: time.Now().Add(-1 * time.Hour).Unix(), // 1 hour ago
	}
	createInvite(t, store, expiredInvite)

	newKey := "new-key-for-expired"
	err := store.InviteRedeem(expiredInvite.PubKey, newKey)

	// Note: The current implementation doesn't check expiration during redemption
	// If expiration checking is added later, this test should expect an error
	expectNoError(t, err, "redeeming expired invite")
}

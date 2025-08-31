package database_test

import (
	"net"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/database"
)

func setupInviteTestDB(t *testing.T) *database.SQLiteStore {
	store, err := database.Init("test-network", ":memory:", false)
	if err != nil {
		t.Fatalf("failed to init test database: %v", err)
	}
	return store
}

func TestInviteCreate(t *testing.T) {
	store := setupInviteTestDB(t)

	tests := []struct {
		name       string
		inviteName string
		pubKey     string
		tempIP     net.IP
		finalIP    net.IP
		admin      bool
		expiration int64
		wantErr    bool
	}{
		{
			name:       "create valid invite",
			inviteName: "test-user",
			pubKey:     "abc123key",
			tempIP:     net.IPv4(10, 0, 64, 1),
			finalIP:    net.IPv4(10, 0, 128, 1),
			admin:      false,
			expiration: time.Now().Add(24 * time.Hour).Unix(),
			wantErr:    false,
		},
		{
			name:       "create admin invite",
			inviteName: "admin-user",
			pubKey:     "xyz789key",
			tempIP:     net.IPv4(10, 0, 64, 2),
			finalIP:    net.IPv4(10, 0, 128, 2),
			admin:      true,
			expiration: time.Now().Add(48 * time.Hour).Unix(),
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.InviteCreate(
				tt.inviteName,
				tt.pubKey,
				tt.tempIP,
				tt.finalIP,
				tt.admin,
				tt.expiration,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("InviteCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Test duplicate name constraint
	t.Run("duplicate name should fail", func(t *testing.T) {
		err := store.InviteCreate(
			"test-user", // same as first test case
			"different-key",
			net.IPv4(10, 0, 64, 3),
			net.IPv4(10, 0, 128, 3),
			false,
			time.Now().Add(24*time.Hour).Unix(),
		)
		if err == nil {
			t.Error("InviteCreate() should fail with duplicate name")
		}
	})

	// Test duplicate public key constraint
	t.Run("duplicate public key should fail", func(t *testing.T) {
		err := store.InviteCreate(
			"different-user",
			"abc123key", // same as first test case
			net.IPv4(10, 0, 64, 4),
			net.IPv4(10, 0, 128, 4),
			false,
			time.Now().Add(24*time.Hour).Unix(),
		)
		if err == nil {
			t.Error("InviteCreate() should fail with duplicate public key")
		}
	})
}

func TestInviteGet(t *testing.T) {
	store := setupInviteTestDB(t)

	// Create test invite
	testName := "test-user"
	testPubKey := "abc123key"
	testTempIP := net.IPv4(10, 0, 64, 1)
	testFinalIP := net.IPv4(10, 0, 128, 1)
	testAdmin := true
	testExpiration := time.Now().Add(24 * time.Hour).Unix()

	err := store.InviteCreate(testName, testPubKey, testTempIP, testFinalIP, testAdmin, testExpiration)
	if err != nil {
		t.Fatalf("failed to create test invite: %v", err)
	}

	t.Run("get existing invite", func(t *testing.T) {
		invite, err := store.InviteGet(testName)
		if err != nil {
			t.Fatalf("InviteGet() error = %v", err)
		}

		if invite.Name != testName {
			t.Errorf("InviteGet() name = %v, want %v", invite.Name, testName)
		}
		if invite.PublicKey != testPubKey {
			t.Errorf("InviteGet() public key = %v, want %v", invite.PublicKey, testPubKey)
		}
		if !invite.Admin {
			t.Errorf("InviteGet() admin = %v, want %v", invite.Admin, testAdmin)
		}
		if invite.Redeemed {
			t.Errorf("InviteGet() redeemed = %v, want false", invite.Redeemed)
		}
		if invite.Expiration.Unix() != testExpiration {
			t.Errorf("InviteGet() expiration = %v, want %v", invite.Expiration.Unix(), testExpiration)
		}

		// Verify CIDR strings are properly formatted
		if invite.InviteCidr == "" {
			t.Error("InviteGet() invite CIDR is empty")
		}
		if invite.NetworkCidr == "" {
			t.Error("InviteGet() network CIDR is empty")
		}
	})

	t.Run("get non-existent invite", func(t *testing.T) {
		_, err := store.InviteGet("non-existent")
		if err == nil {
			t.Error("InviteGet() should fail for non-existent invite")
		}
	})
}

func TestInviteList(t *testing.T) {
	store := setupInviteTestDB(t)

	// Create multiple test invites
	invites := []struct {
		name       string
		pubKey     string
		tempIP     net.IP
		finalIP    net.IP
		admin      bool
		expiration int64
	}{
		{
			name:       "user1",
			pubKey:     "key1",
			tempIP:     net.IPv4(10, 0, 64, 1),
			finalIP:    net.IPv4(10, 0, 128, 1),
			admin:      false,
			expiration: time.Now().Add(24 * time.Hour).Unix(),
		},
		{
			name:       "admin1",
			pubKey:     "key2",
			tempIP:     net.IPv4(10, 0, 64, 2),
			finalIP:    net.IPv4(10, 0, 128, 2),
			admin:      true,
			expiration: time.Now().Add(48 * time.Hour).Unix(),
		},
		{
			name:       "user2",
			pubKey:     "key3",
			tempIP:     net.IPv4(10, 0, 64, 3),
			finalIP:    net.IPv4(10, 0, 128, 3),
			admin:      false,
			expiration: time.Now().Add(72 * time.Hour).Unix(),
		},
	}

	for _, invite := range invites {
		err := store.InviteCreate(
			invite.name,
			invite.pubKey,
			invite.tempIP,
			invite.finalIP,
			invite.admin,
			invite.expiration,
		)
		if err != nil {
			t.Fatalf("failed to create test invite %s: %v", invite.name, err)
		}
	}

	t.Run("list all invites", func(t *testing.T) {
		result, err := store.InviteList()
		if err != nil {
			t.Fatalf("InviteList() error = %v", err)
		}

		if len(result) != len(invites) {
			t.Errorf("InviteList() returned %d invites, want %d", len(result), len(invites))
		}

		// Check that results are ordered by name (as specified in SQL)
		for i := 1; i < len(result); i++ {
			if result[i-1].Name > result[i].Name {
				t.Errorf("InviteList() results not ordered by name: %s > %s", result[i-1].Name, result[i].Name)
			}
		}

		// Verify each invite has required fields populated
		for _, invite := range result {
			if invite.Name == "" {
				t.Error("InviteList() invite has empty name")
			}
			if invite.PublicKey == "" {
				t.Error("InviteList() invite has empty public key")
			}
			if invite.InviteCidr == "" {
				t.Error("InviteList() invite has empty invite CIDR")
			}
			if invite.NetworkCidr == "" {
				t.Error("InviteList() invite has empty network CIDR")
			}
		}
	})

	t.Run("list empty invites", func(t *testing.T) {
		emptyStore := setupInviteTestDB(t)
		result, err := emptyStore.InviteList()
		if err != nil {
			t.Fatalf("InviteList() error = %v", err)
		}

		if len(result) != 0 {
			t.Errorf("InviteList() on empty db returned %d invites, want 0", len(result))
		}
	})
}

func TestInviteRedeem(t *testing.T) {
	store := setupInviteTestDB(t)

	// Create test invite
	testName := "test-user"
	testPubKey := "original-key"
	newPubKey := "new-peer-key"
	testTempIP := net.IPv4(10, 0, 64, 1)
	testFinalIP := net.IPv4(10, 0, 128, 1)
	testAdmin := false
	testExpiration := time.Now().Add(24 * time.Hour).Unix()

	err := store.InviteCreate(testName, testPubKey, testTempIP, testFinalIP, testAdmin, testExpiration)
	if err != nil {
		t.Fatalf("failed to create test invite: %v", err)
	}

	t.Run("redeem valid invite", func(t *testing.T) {
		err := store.InviteRedeem(testPubKey, newPubKey)
		if err != nil {
			t.Fatalf("InviteRedeem() error = %v", err)
		}

		// Verify invite is now marked as redeemed
		invite, err := store.InviteGet(testName)
		if err != nil {
			t.Fatalf("failed to get invite after redemption: %v", err)
		}
		if !invite.Redeemed {
			t.Error("InviteRedeem() invite should be marked as redeemed")
		}

		// Verify peer was created
		peer, err := store.PeerGet(testName)
		if err != nil {
			t.Fatalf("InviteRedeem() should create peer, but got error: %v", err)
		}
		if peer.Name != testName {
			t.Errorf("InviteRedeem() created peer name = %v, want %v", peer.Name, testName)
		}
		if peer.PublicKey != newPubKey {
			t.Errorf("InviteRedeem() created peer key = %v, want %v", peer.PublicKey, newPubKey)
		}
		if peer.Admin != testAdmin {
			t.Errorf("InviteRedeem() created peer admin = %v, want %v", peer.Admin, testAdmin)
		}
		if !peer.Enabled {
			t.Error("InviteRedeem() created peer should be enabled")
		}
		if !peer.Confirmed {
			t.Error("InviteRedeem() created peer should be confirmed")
		}
	})

	t.Run("redeem already redeemed invite", func(t *testing.T) {
		err := store.InviteRedeem(testPubKey, "another-new-key")
		if err == nil {
			t.Error("InviteRedeem() should fail when invite already redeemed")
		}
	})

	t.Run("redeem non-existent invite", func(t *testing.T) {
		err := store.InviteRedeem("non-existent-key", "some-new-key")
		if err == nil {
			t.Error("InviteRedeem() should fail for non-existent invite")
		}
	})

	t.Run("redeem with admin privileges", func(t *testing.T) {
		// Create another invite with admin privileges
		adminName := "admin-user"
		adminPubKey := "admin-original-key"
		adminNewKey := "admin-new-key"
		adminTempIP := net.IPv4(10, 0, 64, 5) // Different IPs to avoid unique constraint
		adminFinalIP := net.IPv4(10, 0, 128, 5)

		err := store.InviteCreate(adminName, adminPubKey, adminTempIP, adminFinalIP, true, testExpiration)
		if err != nil {
			t.Fatalf("failed to create admin test invite: %v", err)
		}

		err = store.InviteRedeem(adminPubKey, adminNewKey)
		if err != nil {
			t.Fatalf("InviteRedeem() admin error = %v", err)
		}

		// Verify admin peer was created with admin privileges
		peer, err := store.PeerGet(adminName)
		if err != nil {
			t.Fatalf("failed to get admin peer after redemption: %v", err)
		}
		if !peer.Admin {
			t.Error("InviteRedeem() admin peer should have admin privileges")
		}
	})
}

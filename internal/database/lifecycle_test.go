package database_test

import (
	"testing"
)

// TestPeerConfirmValid tests confirming a freshly redeemed peer
func TestPeerConfirmValid(t *testing.T) {
	store := setupTestDB(t)

	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating invite")

	newKey := "confirm-peer-key"
	err = store.InviteRedeem(TestUser1.PubKey, newKey)
	expectNoError(t, err, "redeeming invite")

	err = store.PeerConfirm(newKey, TestUser1.FinalIP)
	expectNoError(t, err, "confirming peer")

	assertPeerExists(t, store, TestUser1.Name, newKey, TestUser1.Admin, true)
}

// TestPeerConfirmDeletesInvite tests that confirmation removes the invite record
func TestPeerConfirmDeletesInvite(t *testing.T) {
	store := setupTestDB(t)

	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating invite")

	newKey := "confirm-peer-key"
	err = store.InviteRedeem(TestUser1.PubKey, newKey)
	expectNoError(t, err, "redeeming invite")

	err = store.PeerConfirm(newKey, TestUser1.FinalIP)
	expectNoError(t, err, "confirming peer")

	assertInviteNotExists(t, store, TestUser1.Name)
}

// TestPeerConfirmIdempotent tests that confirming twice succeeds
func TestPeerConfirmIdempotent(t *testing.T) {
	store := setupTestDB(t)

	newKey := "idempotent-peer-key"
	err := createPeerFromInvite(t, store, TestUser1, newKey)
	expectNoError(t, err, "creating confirmed peer")

	err = store.PeerConfirm(newKey, TestUser1.FinalIP)
	expectNoError(t, err, "re-confirming peer")
}

// TestPeerConfirmWrongKey tests that confirmation requires the matching key
func TestPeerConfirmWrongKey(t *testing.T) {
	store := setupTestDB(t)

	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating invite")

	err = store.InviteRedeem(TestUser1.PubKey, "right-key")
	expectNoError(t, err, "redeeming invite")

	err = store.PeerConfirm("wrong-key", TestUser1.FinalIP)
	expectError(t, err, "confirming with wrong key")
}

// TestPeerConfirmWrongIP tests that confirmation requires the assigned IP
func TestPeerConfirmWrongIP(t *testing.T) {
	store := setupTestDB(t)

	err := createInvite(t, store, TestUser1)
	expectNoError(t, err, "creating invite")

	newKey := "wrong-ip-peer-key"
	err = store.InviteRedeem(TestUser1.PubKey, newKey)
	expectNoError(t, err, "redeeming invite")

	err = store.PeerConfirm(newKey, TestUser2.FinalIP)
	expectError(t, err, "confirming from wrong IP")
}

// TestPeerGetByKeyExisting tests looking up a peer by public key
func TestPeerGetByKeyExisting(t *testing.T) {
	store := setupTestDB(t)

	newKey := "lookup-peer-key"
	err := createPeerFromInvite(t, store, TestUser1, newKey)
	expectNoError(t, err, "creating peer")

	peer, err := store.PeerGetByKey(newKey)
	expectNoError(t, err, "getting peer by key")
	if peer.Name != TestUser1.Name {
		t.Errorf("peer name = %v, want %v", peer.Name, TestUser1.Name)
	}
}

// TestPeerGetByKeyNonExistent tests looking up an unknown public key
func TestPeerGetByKeyNonExistent(t *testing.T) {
	store := setupTestDB(t)

	_, err := store.PeerGetByKey("unknown-key")
	expectError(t, err, "getting unknown peer by key")
}

// TestPeerDeleteExisting tests deleting a confirmed peer
func TestPeerDeleteExisting(t *testing.T) {
	store := setupTestDB(t)

	err := createPeerFromInvite(t, store, TestUser1, "delete-peer-key")
	expectNoError(t, err, "creating peer")

	err = store.PeerDelete(TestUser1.Name)
	expectNoError(t, err, "deleting peer")

	assertPeerNotExists(t, store, TestUser1.Name)
}

// TestPeerDeleteNonExistent tests deleting an unknown peer
func TestPeerDeleteNonExistent(t *testing.T) {
	store := setupTestDB(t)

	err := store.PeerDelete("unknown-peer")
	expectError(t, err, "deleting unknown peer")
}

// TestInviteListActiveExcludesRedeemed tests that redeemed invites are filtered
func TestInviteListActiveExcludesRedeemed(t *testing.T) {
	store := setupTestDB(t)

	err := createInvites(t, store, []TestInviteDesc{TestUser1, TestUser2})
	expectNoError(t, err, "creating invites")

	err = store.InviteRedeem(TestUser1.PubKey, "redeemed-key")
	expectNoError(t, err, "redeeming first invite")

	active, err := store.InviteListActive()
	expectNoError(t, err, "listing active invites")
	if len(active) != 1 {
		t.Fatalf("expected 1 active invite, got %d", len(active))
	}
	if active[0].Name != TestUser2.Name {
		t.Errorf("active invite = %v, want %v", active[0].Name, TestUser2.Name)
	}
}

// TestInviteListActiveExcludesExpired tests that expired invites are filtered
func TestInviteListActiveExcludesExpired(t *testing.T) {
	store := setupTestDB(t)

	err := createInvites(t, store, []TestInviteDesc{TestUser1, ExpiredInvite})
	expectNoError(t, err, "creating invites")

	active, err := store.InviteListActive()
	expectNoError(t, err, "listing active invites")
	if len(active) != 1 {
		t.Fatalf("expected 1 active invite, got %d", len(active))
	}
	if active[0].Name != TestUser1.Name {
		t.Errorf("active invite = %v, want %v", active[0].Name, TestUser1.Name)
	}
}

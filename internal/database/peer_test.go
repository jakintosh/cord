package database_test

import (
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// TestPeerGetExisting tests retrieving an existing peer
func TestPeerGetExisting(t *testing.T) {
	store := setupTestDB(t)

	// create valid test peer via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating valid peer")

	// retrieve the peer
	peer, err := store.PeerGet(TestPeer1.Name)
	expectNoError(t, err, "getting existing peer")

	// verify all peer fields match expected values
	if peer.Name != TestPeer1.Name {
		t.Errorf("peer name = %v, want %v", peer.Name, TestPeer1.Name)
	}
	if peer.PublicKey != TestPeer1.PubKey {
		t.Errorf("peer public key = %v, want %v", peer.PublicKey, TestPeer1.PubKey)
	}
	if peer.Admin != TestPeer1.Admin {
		t.Errorf("peer admin = %v, want %v", peer.Admin, TestPeer1.Admin)
	}
	if peer.Enabled != TestPeer1.Enabled {
		t.Errorf("peer enabled = %v, want %v", peer.Enabled, TestPeer1.Enabled)
	}
	if peer.Confirmed != TestPeer1.Confirmed {
		t.Errorf("peer confirmed = %v, want %v", peer.Confirmed, TestPeer1.Confirmed)
	}

	// verify CIDR string is properly formatted
	if peer.Cidr == "" {
		t.Error("peer CIDR is empty")
	}
}

// TestPeerGetNonExistent tests retrieving a non-existent peer
func TestPeerGetNonExistent(t *testing.T) {
	store := setupTestDB(t)

	// attempt to get non-existent peer
	_, err := store.PeerGet("non-existent")
	expectError(t, err, "getting non-existent peer")
}

// TestPeerListMultiple tests listing multiple peers
func TestPeerListMultiple(t *testing.T) {
	store := setupTestDB(t)

	// create multiple test peers via invite redemption
	peers := []TestPeerDesc{TestPeer1, TestPeer2, TestPeerAdmin}
	err := createPeersFromInvites(t, store, peers)
	expectNoError(t, err, "creating multiple peers")

	// list all peers
	result, err := store.PeerList()
	expectNoError(t, err, "listing peers")

	// verify correct number of peers returned
	if len(result) != len(peers) {
		t.Errorf("returned %d peers, want %d", len(result), len(peers))
	}

	// verify results are ordered by name ASC (as specified in SQL)
	for i := 1; i < len(result); i++ {
		if result[i-1].Name > result[i].Name {
			t.Errorf("results not ordered by name ASC: %v > %v",
				result[i-1].Name, result[i].Name)
		}
	}

	// verify each peer has required fields populated
	for _, peer := range result {
		if peer.Name == "" {
			t.Error("peer has empty name")
		}
		if peer.PublicKey == "" {
			t.Error("peer has empty public key")
		}
		if peer.Cidr == "" {
			t.Error("peer has empty CIDR")
		}
	}
}

// TestPeerListEmpty tests listing when no peers exist
func TestPeerListEmpty(t *testing.T) {
	store := setupTestDB(t)

	// list peers from empty database
	result, err := store.PeerList()
	expectNoError(t, err, "listing empty peers")

	// verify no peers returned
	if len(result) != 0 {
		t.Errorf("empty db returned %d peers, want 0", len(result))
	}
	assertPeerCount(t, store, 0)
}

// TestPeerExistsTrue tests PeerExists for existing peer
func TestPeerExistsTrue(t *testing.T) {
	store := setupTestDB(t)

	// create valid test peer via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating valid peer")

	// check if peer exists
	exists := store.PeerExists(TestPeer1.Name)
	if !exists {
		t.Errorf("PeerExists(%s) = false, want true", TestPeer1.Name)
	}
}

// TestPeerExistsFalse tests PeerExists for non-existent peer
func TestPeerExistsFalse(t *testing.T) {
	store := setupTestDB(t)

	// check if non-existent peer exists
	exists := store.PeerExists("non-existent")
	if exists {
		t.Error("PeerExists(non-existent) = true, want false")
	}
}

// TestPeerUpdateName tests updating a peer's name
func TestPeerUpdateName(t *testing.T) {
	store := setupTestDB(t)

	// create valid test peer via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating valid peer")

	// update peer name
	newName := "updated-peer-name"
	updateReq := server.UpdatePeerRequest{
		Name: &newName,
	}
	updatedPeer, err := store.PeerUpdate(TestPeer1.Name, updateReq)
	expectNoError(t, err, "updating peer name")

	// verify name was updated
	if updatedPeer.Name != newName {
		t.Errorf("updated peer name = %v, want %v", updatedPeer.Name, newName)
	}
	// verify other fields remain unchanged
	if updatedPeer.PublicKey != TestPeer1.PubKey {
		t.Errorf("peer public key changed unexpectedly: %v", updatedPeer.PublicKey)
	}
	if updatedPeer.Admin != TestPeer1.Admin {
		t.Errorf("peer admin changed unexpectedly: %v", updatedPeer.Admin)
	}
}

// TestPeerUpdateAdmin tests updating a peer's admin status
func TestPeerUpdateAdmin(t *testing.T) {
	store := setupTestDB(t)

	// create valid test peer via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating valid peer")

	// update peer admin status
	newAdmin := true
	updateReq := server.UpdatePeerRequest{
		Admin: &newAdmin,
	}
	updatedPeer, err := store.PeerUpdate(TestPeer1.Name, updateReq)
	expectNoError(t, err, "updating peer admin status")

	// verify admin status was updated
	if updatedPeer.Admin != newAdmin {
		t.Errorf("updated peer admin = %v, want %v", updatedPeer.Admin, newAdmin)
	}
	// verify other fields remain unchanged
	if updatedPeer.Name != TestPeer1.Name {
		t.Errorf("peer name changed unexpectedly: %v", updatedPeer.Name)
	}
}

// TestPeerUpdateEnabled tests updating a peer's enabled status
func TestPeerUpdateEnabled(t *testing.T) {
	store := setupTestDB(t)

	// create valid test peer via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating valid peer")

	// update peer enabled status
	newEnabled := false
	updateReq := server.UpdatePeerRequest{
		Enabled: &newEnabled,
	}
	updatedPeer, err := store.PeerUpdate(TestPeer1.Name, updateReq)
	expectNoError(t, err, "updating peer enabled status")

	// verify enabled status was updated
	if updatedPeer.Enabled != newEnabled {
		t.Errorf("updated peer enabled = %v, want %v", updatedPeer.Enabled, newEnabled)
	}
	// verify other fields remain unchanged
	if updatedPeer.Name != TestPeer1.Name {
		t.Errorf("peer name changed unexpectedly: %v", updatedPeer.Name)
	}
}

// TestPeerUpdateMultipleFields tests updating multiple peer fields at once
func TestPeerUpdateMultipleFields(t *testing.T) {
	store := setupTestDB(t)

	// create valid test peer via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating valid peer")

	// update multiple fields
	newName := "multi-updated-peer"
	newAdmin := true
	newEnabled := false
	updateReq := server.UpdatePeerRequest{
		Name:    &newName,
		Admin:   &newAdmin,
		Enabled: &newEnabled,
	}
	updatedPeer, err := store.PeerUpdate(TestPeer1.Name, updateReq)
	expectNoError(t, err, "updating multiple peer fields")

	// verify all fields were updated
	if updatedPeer.Name != newName {
		t.Errorf("updated peer name = %v, want %v", updatedPeer.Name, newName)
	}
	if updatedPeer.Admin != newAdmin {
		t.Errorf("updated peer admin = %v, want %v", updatedPeer.Admin, newAdmin)
	}
	if updatedPeer.Enabled != newEnabled {
		t.Errorf("updated peer enabled = %v, want %v", updatedPeer.Enabled, newEnabled)
	}
	// verify unchanged fields
	if updatedPeer.PublicKey != TestPeer1.PubKey {
		t.Errorf("peer public key changed unexpectedly: %v", updatedPeer.PublicKey)
	}
}

// TestPeerUpdateNonExistent tests updating a non-existent peer
func TestPeerUpdateNonExistent(t *testing.T) {
	store := setupTestDB(t)

	// attempt to update non-existent peer
	newName := "does-not-matter"
	updateReq := server.UpdatePeerRequest{
		Name: &newName,
	}
	_, err := store.PeerUpdate("non-existent", updateReq)
	expectError(t, err, "updating non-existent peer")
}

// TestPeerListPeersWithAssociations tests the complex PeerListPeers function
// This requires some setup with CIDRs and associations to work properly
func TestPeerListPeersEmpty(t *testing.T) {
	store := setupTestDB(t)

	// create a test peer but no associations via invite redemption
	inviteDesc := peerDescToInviteDesc(TestPeer1)
	err := createPeerFromInvite(t, store, inviteDesc, TestPeer1.PubKey)
	expectNoError(t, err, "creating test peer")

	// attempt to list peers for this peer (should return empty since no associations exist)
	result, err := store.PeerListPeers(TestPeer1.Name)
	expectNoError(t, err, "listing peers for peer with no associations")

	// should return empty list since no CIDRs/associations are set up
	if len(result) != 0 {
		t.Errorf("expected 0 peers for peer with no associations, got %d", len(result))
	}
}

// TestPeerGetByIPValid tests retrieving a peer by IP when enabled and confirmed
func TestPeerGetByIPValid(t *testing.T) {
	store := setupTestDB(t)

	peerPub := "peer-public-valid"
	err := createPeerFromInvite(t, store, TestUser1, peerPub)
	expectNoError(t, err, "creating peer for GetByIP")

	// Lookup by IP
	got, err := store.PeerGetByIP(TestUser1.FinalIP)
	expectNoError(t, err, "PeerGetByIP should find enabled+confirmed peer")
	if got == nil {
		t.Fatalf("PeerGetByIP returned nil peer")
	}
	if got.Name != TestUser1.Name {
		t.Errorf("peer name = %v, want %v", got.Name, TestUser1.Name)
	}
	if got.PublicKey != peerPub {
		t.Errorf("peer public key = %v, want %v", got.PublicKey, peerPub)
	}
}

// TestPeerGetByIPDisabledNotReturned ensures disabled peers are not returned by IP
func TestPeerGetByIPDisabledNotReturned(t *testing.T) {
	store := setupTestDB(t)

	// Create peer via invite redemption with explicit IPv4 addresses (4-byte)
	peerPub := "peer-public-disabled"
	err := createPeerFromInvite(t, store, TestUser1, peerPub)
	expectNoError(t, err, "creating peer to disable")

	// Disable the peer
	update := server.UpdatePeerRequest{Enabled: boolPtr(false)}
	_, err = store.PeerUpdate(TestUser1.Name, update)
	expectNoError(t, err, "disabling peer")

	// Lookup by IP should fail
	_, err = store.PeerGetByIP(TestAdmin.FinalIP)
	expectError(t, err, "PeerGetByIP should not return disabled peer")
}

// local helper for pointer to bool
func boolPtr(v bool) *bool { return &v }

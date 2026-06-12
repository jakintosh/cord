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

// TestPeerDesc describes a peer for testing
type TestPeerDesc struct {
	Name      string
	PubKey    string
	IP        net.IP
	Prefix    int
	Admin     bool
	Enabled   bool
	Confirmed bool
}

// TestCidrDesc describes a CIDR for testing
type TestCidrDesc struct {
	Name   string
	Cidr   string
	Length int
	Prefix int
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

// Common test peers
var (
	TestPeer1 = TestPeerDesc{
		Name:      "test-peer-1",
		PubKey:    "peer1-public-key",
		IP:        net.IPv4(10, 0, 128, 1),
		Prefix:    32,
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}

	TestPeer2 = TestPeerDesc{
		Name:      "test-peer-2",
		PubKey:    "peer2-public-key",
		IP:        net.IPv4(10, 0, 128, 2),
		Prefix:    32,
		Admin:     false,
		Enabled:   true,
		Confirmed: false,
	}

	TestPeerAdmin = TestPeerDesc{
		Name:      "admin-peer",
		PubKey:    "admin-public-key",
		IP:        net.IPv4(10, 0, 128, 10),
		Prefix:    32,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}
)

// Common test CIDRs
var (
	TestCidrRoot = TestCidrDesc{
		Name:   "test-network",
		Cidr:   "10.0.0.0/16",
		Length: 32,
		Prefix: 16,
	}

	TestCidr1 = TestCidrDesc{
		Name:   "subnet-1",
		Cidr:   "10.0.64.0/24",
		Length: 32,
		Prefix: 24,
	}

	TestCidr2 = TestCidrDesc{
		Name:   "subnet-2",
		Cidr:   "10.0.65.0/24",
		Length: 32,
		Prefix: 24,
	}

	TestCidrSmall = TestCidrDesc{
		Name:   "small-subnet",
		Cidr:   "10.0.66.0/28",
		Length: 32,
		Prefix: 28,
	}
)

// setupTestDB creates a new in-memory SQLite database for testing
func setupTestDB(t *testing.T) *database.ServerDB {
	t.Helper()
	store, err := database.OpenServer(database.Options{
		Name: "test-network",
		Dir:  ":memory:",
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// createInvite creates an invite from a TestInviteDesc
func createInvite(
	t *testing.T,
	store *database.ServerDB,
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
	store *database.ServerDB,
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
	store *database.ServerDB,
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
	store *database.ServerDB,
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
	store *database.ServerDB,
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
	store *database.ServerDB,
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
	store *database.ServerDB,
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
	store *database.ServerDB,
	name, pubKey string,
	admin bool,
	confirmed bool,
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
	if peer.Confirmed != confirmed {
		t.Errorf("peer confirmed = %v, want %v", peer.Confirmed, confirmed)
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

// createPeerFromInvite creates a confirmed peer by redeeming an invite
// and confirming the resulting peer, mirroring the full join flow
func createPeerFromInvite(
	t *testing.T,
	store *database.ServerDB,
	inviteDesc TestInviteDesc,
	newPubKey string,
) error {
	t.Helper()

	// First create the invite
	err := createInvite(t, store, inviteDesc)
	if err != nil {
		return err
	}

	// Then redeem it to create the (unconfirmed) peer
	if err := store.InviteRedeem(inviteDesc.PubKey, newPubKey); err != nil {
		return err
	}

	// Finally confirm the peer at its assigned IP
	return store.PeerConfirm(newPubKey, inviteDesc.FinalIP)
}

// peerDescToInviteDesc converts a TestPeerDesc to TestInviteDesc for invite creation
func peerDescToInviteDesc(desc TestPeerDesc) TestInviteDesc {
	// Use different temp IP based on final IP to avoid conflicts
	tempIP := make(net.IP, len(desc.IP))
	copy(tempIP, desc.IP)
	if len(tempIP) >= 3 {
		tempIP[2] = 64 // change third octet to 64 for temp IP
	}

	return TestInviteDesc{
		Name:       desc.Name,
		PubKey:     "invite-" + desc.PubKey, // different key for invite
		TempIP:     tempIP,                  // unique temp IP
		FinalIP:    desc.IP,                 // final IP from peer desc
		Admin:      desc.Admin,
		Expiration: time.Now().Add(24 * time.Hour).Unix(),
	}
}

// createPeersFromInvites creates multiple peers by redeeming invites
func createPeersFromInvites(
	t *testing.T,
	store *database.ServerDB,
	descs []TestPeerDesc,
) error {
	t.Helper()
	for _, desc := range descs {
		inviteDesc := peerDescToInviteDesc(desc)
		if err := createPeerFromInvite(t, store, inviteDesc, desc.PubKey); err != nil {
			return err
		}
	}
	return nil
}

// assertPeerCount verifies the total number of peers
func assertPeerCount(
	t *testing.T,
	store *database.ServerDB,
	expectedCount int,
) {
	t.Helper()
	peers, err := store.PeerList()
	if err != nil {
		t.Fatalf("failed to list peers: %v", err)
	}
	if len(peers) != expectedCount {
		t.Errorf("expected %d peers, got %d", expectedCount, len(peers))
	}
}

// assertPeerNotExists verifies that a peer does not exist
func assertPeerNotExists(
	t *testing.T,
	store *database.ServerDB,
	name string,
) {
	t.Helper()
	if store.PeerExists(name) {
		t.Errorf("expected peer %s to not exist, but it was found", name)
	}
}

// createCidr creates a CIDR from a TestCidrDesc
func createCidr(
	t *testing.T,
	store *database.ServerDB,
	desc TestCidrDesc,
) error {
	t.Helper()
	_, cidr, err := net.ParseCIDR(desc.Cidr)
	if err != nil {
		t.Fatalf("failed to parse test CIDR %s: %v", desc.Cidr, err)
	}
	return store.CidrCreate(desc.Name, cidr)
}

// createRootCidr creates a root CIDR from a TestCidrDesc
func createRootCidr(
	t *testing.T,
	store *database.ServerDB,
	desc TestCidrDesc,
) error {
	t.Helper()
	_, cidr, err := net.ParseCIDR(desc.Cidr)
	if err != nil {
		t.Fatalf("failed to parse test CIDR %s: %v", desc.Cidr, err)
	}
	return store.CidrCreateRoot(desc.Name, cidr)
}

// createCidrs creates multiple CIDRs from TestCidrDesc slice
func createCidrs(
	t *testing.T,
	store *database.ServerDB,
	descs []TestCidrDesc,
) error {
	t.Helper()
	for _, desc := range descs {
		if err := createCidr(t, store, desc); err != nil {
			return err
		}
	}
	return nil
}

// assertCidrExists verifies that a CIDR exists and matches expected values
func assertCidrExists(
	t *testing.T,
	store *database.ServerDB,
	desc TestCidrDesc,
) {
	t.Helper()
	cidr, err := store.CidrGet(desc.Name)
	if err != nil {
		t.Fatalf("expected CIDR %s to exist, but got error: %v", desc.Name, err)
	}

	if cidr.Name != desc.Name {
		t.Errorf("CIDR name = %v, want %v", cidr.Name, desc.Name)
	}
	if cidr.Cidr != desc.Cidr {
		t.Errorf("CIDR cidr = %v, want %v", cidr.Cidr, desc.Cidr)
	}
	if cidr.Length != desc.Length {
		t.Errorf("CIDR length = %v, want %v", cidr.Length, desc.Length)
	}
	if cidr.Prefix != desc.Prefix {
		t.Errorf("CIDR prefix = %v, want %v", cidr.Prefix, desc.Prefix)
	}
}

// assertCidrNotExists verifies that a CIDR does not exist
func assertCidrNotExists(
	t *testing.T,
	store *database.ServerDB,
	name string,
) {
	t.Helper()
	_, err := store.CidrGet(name)
	if err == nil {
		t.Errorf("expected CIDR %s to not exist, but it was found", name)
	}
}

// assertCidrCount verifies the total number of CIDRs
func assertCidrCount(
	t *testing.T,
	store *database.ServerDB,
	expectedCount int,
) {
	t.Helper()
	cidrs, err := store.CidrList()
	if err != nil {
		t.Fatalf("failed to list CIDRs: %v", err)
	}
	if len(cidrs) != expectedCount {
		t.Errorf("expected %d CIDRs, got %d", expectedCount, len(cidrs))
	}
}

// createAssociation creates an association between two CIDR names
func createAssociation(
	t *testing.T,
	store *database.ServerDB,
	cidr1, cidr2 string,
) error {
	t.Helper()
	return store.AssociationCreate(cidr1, cidr2)
}

// createAssociations creates multiple associations from pairs of CIDR names
func createAssociations(
	t *testing.T,
	store *database.ServerDB,
	pairs [][2]string,
) error {
	t.Helper()
	for _, pair := range pairs {
		if err := createAssociation(t, store, pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

// assertAssociationExists verifies that an association exists between two CIDRs
func assertAssociationExists(
	t *testing.T,
	store *database.ServerDB,
	cidr1, cidr2 string,
) {
	t.Helper()
	associations, err := store.AssociationList()
	if err != nil {
		t.Fatalf("failed to list associations: %v", err)
	}

	for _, assoc := range associations {
		if (assoc.Cidr1 == cidr1 && assoc.Cidr2 == cidr2) ||
			(assoc.Cidr1 == cidr2 && assoc.Cidr2 == cidr1) {
			return // found the association
		}
	}
	t.Errorf("expected association between %s and %s to exist, but it was not found", cidr1, cidr2)
}

// assertAssociationNotExists verifies that an association does not exist between two CIDRs
func assertAssociationNotExists(
	t *testing.T,
	store *database.ServerDB,
	cidr1, cidr2 string,
) {
	t.Helper()
	associations, err := store.AssociationList()
	if err != nil {
		t.Fatalf("failed to list associations: %v", err)
	}

	for _, assoc := range associations {
		if (assoc.Cidr1 == cidr1 && assoc.Cidr2 == cidr2) ||
			(assoc.Cidr1 == cidr2 && assoc.Cidr2 == cidr1) {
			t.Errorf("expected association between %s and %s to not exist, but it was found", cidr1, cidr2)
			return
		}
	}
}

// assertAssociationCount verifies the total number of associations
func assertAssociationCount(
	t *testing.T,
	store *database.ServerDB,
	expectedCount int,
) {
	t.Helper()
	associations, err := store.AssociationList()
	if err != nil {
		t.Fatalf("failed to list associations: %v", err)
	}
	if len(associations) != expectedCount {
		t.Errorf("expected %d associations, got %d", expectedCount, len(associations))
	}
}

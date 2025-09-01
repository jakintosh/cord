package database_test

import (
	"net"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/utils"
)

// TestCreateNetworkAtomic_Success verifies that Create() atomically creates the
// root CIDR and the initial server peer with correct attributes.
func TestCreateNetworkAtomic_Success(t *testing.T) {
	store := setupTestDB(t)

	// Prepare inputs
	_, root, err := net.ParseCIDR(TestCidrRoot.Cidr)
	if err != nil {
		t.Fatalf("failed to parse root cidr: %v", err)
	}
	serverPubKey := "server-public-key-123"

	// Execute
	if err := store.Create(TestCidrRoot.Name, root, serverPubKey); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Validate CIDR exists
	assertCidrExists(t, store, TestCidrRoot)

	// Validate server peer exists with expected fields
	assertPeerExists(t, store, "cord-server", serverPubKey, true)

	// Validate server peer IP and prefix derived from root
	peer, err := store.PeerGet("cord-server")
	if err != nil {
		t.Fatalf("failed to get server peer: %v", err)
	}

	firstIP := utils.GetFirstAssignableIpFromCidr(root)
	expectedCIDR := &net.IPNet{IP: firstIP, Mask: net.CIDRMask(32, 32)}
	if peer.Cidr != expectedCIDR.String() {
		t.Errorf("server peer cidr = %s, want %s", peer.Cidr, expectedCIDR.String())
	}
}

// TestCreateNetworkAtomic_Duplicate ensures a second Create() returns an error
// and prior state remains intact.
func TestCreateNetworkAtomic_Duplicate(t *testing.T) {
	store := setupTestDB(t)

	_, root, err := net.ParseCIDR(TestCidrRoot.Cidr)
	if err != nil {
		t.Fatalf("failed to parse root cidr: %v", err)
	}

	if err := store.Create(TestCidrRoot.Name, root, "server-pub-1"); err != nil {
		t.Fatalf("initial Create() failed: %v", err)
	}

	// Attempt duplicate create with different pubkey
	if err := store.Create(TestCidrRoot.Name, root, "server-pub-2"); err == nil {
		t.Fatalf("expected duplicate Create() to fail, but it succeeded")
	}

	// Ensure prior state intact: 1 cidr (root) and 1 peer (cord-server)
	assertCidrExists(t, store, TestCidrRoot)
	assertPeerExists(t, store, "cord-server", "server-pub-1", true)
}

// TestCreateNetworkAtomic_IPv6 validates prefix correctness for IPv6.
func TestCreateNetworkAtomic_IPv6(t *testing.T) {
	store := setupTestDB(t)

	_, root, err := net.ParseCIDR("fd00::/64")
	if err != nil {
		t.Fatalf("failed to parse ipv6 root cidr: %v", err)
	}

	if err := store.Create("ipv6-network", root, "server-pub-v6"); err != nil {
		t.Fatalf("Create() failed for ipv6: %v", err)
	}

	peer, err := store.PeerGet("cord-server")
	if err != nil {
		t.Fatalf("failed to get server peer: %v", err)
	}

	// Expect /128 prefix and first assignable IP from fd00::/64
	firstIP := utils.GetFirstAssignableIpFromCidr(root)
	expectedCIDR := &net.IPNet{IP: firstIP, Mask: net.CIDRMask(128, 128)}
	if peer.Cidr != expectedCIDR.String() {
		t.Errorf("server peer cidr = %s, want %s", peer.Cidr, expectedCIDR.String())
	}
}

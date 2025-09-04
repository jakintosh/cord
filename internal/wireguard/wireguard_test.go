package wireguard_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// TestInterfaceDesc describes a WireGuard interface for testing
type TestInterfaceDesc struct {
	Name       string
	Address    string // e.g., "10.0.0.1/24"
	ListenPort int
}

// TestPeerDesc describes a WireGuard peer for testing
type TestPeerDesc struct {
	Name                string
	AllowedIPs          []string // e.g., []string{"10.0.0.2/32"}
	Endpoint            string   // e.g., "192.168.1.100:51820" (empty = no endpoint)
	PersistentKeepalive int      // seconds (0 = no keepalive)
}

// Pre-configured test data
var (
	TestInterface = TestInterfaceDesc{
		Name:       "cord-test",
		Address:    "10.0.0.1/24",
		ListenPort: 51820,
	}

	TestInterfaceNoPort = TestInterfaceDesc{
		Name:       "cord-empty",
		Address:    "10.0.0.1/24",
		ListenPort: 0, // No listen port
	}

	TestPeerWithEndpoint = TestPeerDesc{
		Name:                "peer1",
		AllowedIPs:          []string{"10.0.0.2/32"},
		Endpoint:            "192.168.1.100:51820",
		PersistentKeepalive: 25,
	}

	TestPeerMinimal = TestPeerDesc{
		Name:       "peer2",
		AllowedIPs: []string{"10.0.0.3/32"},
		// No endpoint, no keepalive
	}

	TestPeerMultipleIPs = TestPeerDesc{
		Name:       "peer-multi",
		AllowedIPs: []string{"10.0.0.2/32", "192.168.1.0/24"},
	}
)

// createTestInterface creates an Interface from a TestInterfaceDesc and TestPeerDescs
func createTestInterface(
	t *testing.T,
	ifaceDesc TestInterfaceDesc,
	peerDescs []TestPeerDesc,
) *wireguard.Interface {
	t.Helper()

	// Generate private key for interface
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	// Parse interface address
	interfaceIP := net.ParseIP(strings.Split(ifaceDesc.Address, "/")[0])
	_, network, err := net.ParseCIDR(ifaceDesc.Address)
	if err != nil {
		t.Fatalf("failed to parse interface CIDR %s: %v", ifaceDesc.Address, err)
	}
	interfaceNet := &net.IPNet{
		IP:   interfaceIP,
		Mask: network.Mask,
	}

	// Create peers
	var peers []wireguard.Peer
	for _, peerDesc := range peerDescs {
		peer := createTestPeer(t, peerDesc)
		peers = append(peers, peer)
	}

	return &wireguard.Interface{
		Name:       ifaceDesc.Name,
		PrivateKey: privateKey,
		Address:    *interfaceNet,
		ListenPort: ifaceDesc.ListenPort,
		Peers:      peers,
	}
}

// createTestPeer creates a Peer from a TestPeerDesc
func createTestPeer(
	t *testing.T,
	desc TestPeerDesc,
) wireguard.Peer {
	t.Helper()

	// Generate key for peer
	peerKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate peer key for %s: %v", desc.Name, err)
	}

	// Parse allowed IPs
	var allowedIPs []*net.IPNet
	for _, allowedIPStr := range desc.AllowedIPs {
		_, allowedIP, err := net.ParseCIDR(allowedIPStr)
		if err != nil {
			t.Fatalf("failed to parse allowed IP %s for peer %s: %v", allowedIPStr, desc.Name, err)
		}
		allowedIPs = append(allowedIPs, allowedIP)
	}

	// Parse endpoint if specified
	var endpoint *net.UDPAddr
	if desc.Endpoint != "" {
		endpoint, err = net.ResolveUDPAddr("udp", desc.Endpoint)
		if err != nil {
			t.Fatalf("failed to resolve endpoint %s for peer %s: %v", desc.Endpoint, desc.Name, err)
		}
	}

	return wireguard.Peer{
		PublicKey:           peerKey.PublicKey(),
		AllowedIPs:          allowedIPs,
		Endpoint:            endpoint,
		PersistentKeepalive: time.Duration(desc.PersistentKeepalive) * time.Second,
	}
}

// assertConfigContains verifies config contains expected content
func assertConfigContains(
	t *testing.T,
	config string,
	expected string,
) {
	t.Helper()
	if !strings.Contains(config, expected) {
		t.Errorf("config missing expected content: %s", expected)
	}
}

// assertConfigNotContains verifies config does not contain content
func assertConfigNotContains(
	t *testing.T,
	config string,
	content string,
) {
	t.Helper()
	if strings.Contains(config, content) {
		t.Errorf("config should not contain: %s", content)
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

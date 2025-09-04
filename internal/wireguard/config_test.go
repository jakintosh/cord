package wireguard_test

import (
	"fmt"
	"strings"
	"testing"
)

// TestInterface_ToWgConfig tests generating config with multiple peer types
func TestInterface_ToWgConfig(t *testing.T) {
	// Create interface with peer that has endpoint and minimal peer
	iface := createTestInterface(t, TestInterface, []TestPeerDesc{
		TestPeerWithEndpoint,
		TestPeerMinimal,
	})

	config := iface.ToWgConfig()

	// Verify interface configuration
	assertConfigContains(t, config, "[Interface]")
	assertConfigContains(t, config, fmt.Sprintf("PrivateKey = %s", iface.PrivateKey.String()))
	assertConfigContains(t, config, "Address = 10.0.0.1/24")
	assertConfigContains(t, config, "ListenPort = 51820")

	// Verify peer configurations
	assertConfigContains(t, config, "[Peer]")
	assertConfigContains(t, config, fmt.Sprintf("PublicKey = %s", iface.Peers[0].PublicKey.String()))
	assertConfigContains(t, config, fmt.Sprintf("PublicKey = %s", iface.Peers[1].PublicKey.String()))
	assertConfigContains(t, config, "AllowedIPs = 10.0.0.2/32")
	assertConfigContains(t, config, "AllowedIPs = 10.0.0.3/32")
	assertConfigContains(t, config, "Endpoint = 192.168.1.100:51820")
	assertConfigContains(t, config, "PersistentKeepalive = 25")

	// Verify peer2 doesn't have endpoint or keepalive in its section
	lines := strings.Split(config, "\n")
	peer2Section := false
	for _, line := range lines {
		if strings.Contains(line, fmt.Sprintf("PublicKey = %s", iface.Peers[1].PublicKey.String())) {
			peer2Section = true
			continue
		}
		if peer2Section && strings.HasPrefix(line, "[") {
			break // moved to next section
		}
		if peer2Section {
			if strings.Contains(line, "Endpoint =") {
				t.Error("peer2 should not have endpoint in config")
			}
			if strings.Contains(line, "PersistentKeepalive =") {
				t.Error("peer2 should not have persistent keepalive in config")
			}
		}
	}
}

// TestInterface_ToWgConfig_EmptyInterface tests generating config for interface with no peers
func TestInterface_ToWgConfig_EmptyInterface(t *testing.T) {
	// Create interface with no peers and no listen port
	iface := createTestInterface(t, TestInterfaceNoPort, []TestPeerDesc{})

	config := iface.ToWgConfig()

	// Should have interface section but no listen port or peers
	assertConfigContains(t, config, "[Interface]")
	assertConfigContains(t, config, fmt.Sprintf("PrivateKey = %s", iface.PrivateKey.String()))
	assertConfigContains(t, config, "Address = 10.0.0.1/24")

	// Should not have ListenPort when port is 0
	assertConfigNotContains(t, config, "ListenPort =")

	// Should not have any peer sections
	assertConfigNotContains(t, config, "[Peer]")
}

// TestInterface_ToWgConfig_MultipleAllowedIPs tests peer with multiple allowed IP ranges
func TestInterface_ToWgConfig_MultipleAllowedIPs(t *testing.T) {
	// Create interface with peer that has multiple allowed IPs
	iface := createTestInterface(t, TestInterface, []TestPeerDesc{
		TestPeerMultipleIPs,
	})

	config := iface.ToWgConfig()

	// Should contain multiple allowed IPs separated by comma and space
	assertConfigContains(t, config, "AllowedIPs = 10.0.0.2/32, 192.168.1.0/24")
}

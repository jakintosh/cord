package wireguard

import (
	"encoding/hex"
	"net"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func mustGenerateKey(t *testing.T) wgtypes.Key {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func mustParseCIDR(t *testing.T, s string) net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse CIDR %q: %v", s, err)
	}
	return *n
}

func mustResolveUDP(t *testing.T, s string) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		t.Fatalf("resolve UDP %q: %v", s, err)
	}
	return addr
}

func TestParseUAPIPeers_Empty(t *testing.T) {
	peers, err := parseUAPIPeers("")
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestParseUAPIPeers_IgnoresListenPort(t *testing.T) {
	raw := "listen_port=51820\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestParseUAPIPeers_SinglePeer(t *testing.T) {
	k := mustGenerateKey(t)
	keyHex := hex.EncodeToString(k[:])
	raw := "public_key=" + keyHex + "\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].PublicKey != k {
		t.Error("public key mismatch")
	}
}

func TestParseUAPIPeers_PeerWithEndpoint(t *testing.T) {
	k := mustGenerateKey(t)
	keyHex := hex.EncodeToString(k[:])
	raw := "public_key=" + keyHex + "\nendpoint=1.2.3.4:51820\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	peer := peers[0]
	if peer.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", peer.Endpoint)
	}
}

func TestParseUAPIPeers_PeerWithHandshake(t *testing.T) {
	k := mustGenerateKey(t)
	keyHex := hex.EncodeToString(k[:])
	raw := "public_key=" + keyHex + "\nlast_handshake_time_sec=1700000000\nlast_handshake_time_nsec=500000000\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	peer := peers[0]
	expected := time.Unix(1700000000, 500000000)
	if !peer.LastHandshake.Equal(expected) {
		t.Errorf("last handshake = %v, want %v", peer.LastHandshake, expected)
	}
}

func TestParseUAPIPeers_PeerWithKeepalive(t *testing.T) {
	k := mustGenerateKey(t)
	keyHex := hex.EncodeToString(k[:])
	raw := "public_key=" + keyHex + "\npersistent_keepalive_interval=30\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if peers[0].PersistentKeepalive != 30*time.Second {
		t.Errorf("keepalive = %v, want 30s", peers[0].PersistentKeepalive)
	}
}

func TestParseUAPIPeers_PeerWithAllowedIPs(t *testing.T) {
	k := mustGenerateKey(t)
	keyHex := hex.EncodeToString(k[:])
	raw := "public_key=" + keyHex + "\nallowed_ip=10.0.0.1/32\nallowed_ip=10.0.1.0/24\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	peer := peers[0]
	if len(peer.AllowedIPs) != 2 {
		t.Fatalf("expected 2 allowed IPs, got %d", len(peer.AllowedIPs))
	}
	if peer.AllowedIPs[0].String() != "10.0.0.1/32" {
		t.Errorf("first IP = %s, want 10.0.0.1/32", peer.AllowedIPs[0].String())
	}
	if peer.AllowedIPs[1].String() != "10.0.1.0/24" {
		t.Errorf("second IP = %s, want 10.0.1.0/24", peer.AllowedIPs[1].String())
	}
}

func TestParseUAPIPeers_PeerWithCounters(t *testing.T) {
	k := mustGenerateKey(t)
	keyHex := hex.EncodeToString(k[:])
	raw := "public_key=" + keyHex + "\nrx_bytes=1024\ntx_bytes=2048\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	peer := peers[0]
	if peer.ReceiveBytes != 1024 {
		t.Errorf("rx_bytes = %d, want 1024", peer.ReceiveBytes)
	}
	if peer.TransmitBytes != 2048 {
		t.Errorf("tx_bytes = %d, want 2048", peer.TransmitBytes)
	}
}

func TestParseUAPIPeers_MultiplePeers(t *testing.T) {
	k1, k2 := mustGenerateKey(t), mustGenerateKey(t)
	raw := "public_key=" + hex.EncodeToString(k1[:]) + "\n" +
		"public_key=" + hex.EncodeToString(k2[:]) + "\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}
}

func TestParseUAPIPeers_InvalidPublicKey(t *testing.T) {
	raw := "public_key=nothex\n"
	_, err := parseUAPIPeers(raw)
	if err == nil {
		t.Error("expected error for invalid public key")
	}
}

func TestParseUAPIPeers_WrongLengthPublicKey(t *testing.T) {
	raw := "public_key=" + strings.Repeat("00", 16) + "\n"
	_, err := parseUAPIPeers(raw)
	if err == nil {
		t.Error("expected error for wrong-length public key")
	}
}

func TestParseUAPIPeers_SkipsLinesWithoutEquals(t *testing.T) {
	raw := "garbage\nlisten_port=12345\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestParseUAPIPeers_FlushBetweenPeers(t *testing.T) {
	k1, k2 := mustGenerateKey(t), mustGenerateKey(t)
	raw := "public_key=" + hex.EncodeToString(k1[:]) + "\n" +
		"last_handshake_time_sec=100\nlast_handshake_time_nsec=0\n" +
		"public_key=" + hex.EncodeToString(k2[:]) + "\n" +
		"last_handshake_time_sec=200\nlast_handshake_time_nsec=0\n"
	peers, err := parseUAPIPeers(raw)
	if err != nil {
		t.Fatalf("parseUAPIPeers: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
	if !peers[0].LastHandshake.Equal(time.Unix(100, 0)) {
		t.Errorf("peer 0 handshake = %v, want 100", peers[0].LastHandshake)
	}
	if !peers[1].LastHandshake.Equal(time.Unix(200, 0)) {
		t.Errorf("peer 1 handshake = %v, want 200", peers[1].LastHandshake)
	}
}

func TestPeerOperationsUAPI_Add(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type: PeerAdd,
		Peer: Peer{
			PublicKey:           k,
			AllowedIPs:          []net.IPNet{mustParseCIDR(t, "10.0.0.1/32")},
			EndpointPolicy:      EndpointFixed,
			Endpoint:            mustResolveUDP(t, "1.2.3.4:51820"),
			PersistentKeepalive: 30 * time.Second,
		},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if !strings.Contains(result, "public_key=") {
		t.Error("missing public_key line")
	}
	if !strings.Contains(result, "replace_allowed_ips=true") {
		t.Error("missing replace_allowed_ips")
	}
	if !strings.Contains(result, "allowed_ip=10.0.0.1/32") {
		t.Error("missing allowed_ip")
	}
	if !strings.Contains(result, "endpoint=1.2.3.4:51820") {
		t.Error("missing endpoint")
	}
	if !strings.Contains(result, "persistent_keepalive_interval=30") {
		t.Error("missing keepalive")
	}
}

func TestPeerOperationsUAPI_AddDynamicEndpoint(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type: PeerAdd,
		Peer: Peer{
			PublicKey:           k,
			AllowedIPs:          []net.IPNet{mustParseCIDR(t, "10.0.0.1/32")},
			EndpointPolicy:      EndpointDynamic,
			Endpoint:            mustResolveUDP(t, "1.2.3.4:51820"),
			PersistentKeepalive: 0,
		},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if strings.Contains(result, "endpoint=") {
		t.Error("should not contain endpoint for EndpointDynamic")
	}
}

func TestPeerOperationsUAPI_Remove(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type: PeerRemove,
		Peer: Peer{PublicKey: k},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if !strings.Contains(result, "remove=true") {
		t.Error("missing remove=true")
	}
	if strings.Contains(result, "allowed_ip") {
		t.Error("remove should not contain allowed IPs")
	}
}

func TestPeerOperationsUAPI_Update_AllowedIPs(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type:             PeerUpdate,
		UpdateAllowedIPs: true,
		Peer: Peer{
			PublicKey:  k,
			AllowedIPs: []net.IPNet{mustParseCIDR(t, "10.0.1.0/24")},
		},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if !strings.Contains(result, "update_only=true") {
		t.Error("missing update_only")
	}
	if !strings.Contains(result, "replace_allowed_ips=true") {
		t.Error("missing replace_allowed_ips")
	}
	if !strings.Contains(result, "allowed_ip=10.0.1.0/24") {
		t.Error("missing allowed_ip")
	}
}

func TestPeerOperationsUAPI_Update_Endpoint(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type:           PeerUpdate,
		UpdateEndpoint: true,
		Peer: Peer{
			PublicKey: k,
			Endpoint:  mustResolveUDP(t, "5.6.7.8:51821"),
		},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if !strings.Contains(result, "endpoint=5.6.7.8:51821") {
		t.Error("missing endpoint")
	}
}

func TestPeerOperationsUAPI_Update_Keepalive(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type:            PeerUpdate,
		UpdateKeepalive: true,
		Peer: Peer{
			PublicKey:           k,
			PersistentKeepalive: 15 * time.Second,
		},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if !strings.Contains(result, "persistent_keepalive_interval=15") {
		t.Error("missing keepalive")
	}
}

func TestPeerOperationsUAPI_Update_MultipleFields(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{
		Type:             PeerUpdate,
		UpdateAllowedIPs: true,
		UpdateEndpoint:   true,
		UpdateKeepalive:  true,
		Peer: Peer{
			PublicKey:           k,
			AllowedIPs:          []net.IPNet{mustParseCIDR(t, "10.0.0.0/16")},
			Endpoint:            mustResolveUDP(t, "1.1.1.1:1111"),
			PersistentKeepalive: 60 * time.Second,
		},
	}

	result := peerOperationsUAPI([]PeerOperation{op})
	if !strings.Contains(result, "update_only=true") {
		t.Error("missing update_only")
	}
	if !strings.Contains(result, "allowed_ip=10.0.0.0/16") {
		t.Error("missing allowed_ip")
	}
	if !strings.Contains(result, "endpoint=1.1.1.1:1111") {
		t.Error("missing endpoint")
	}
	if !strings.Contains(result, "persistent_keepalive_interval=60") {
		t.Error("missing keepalive")
	}
}

func TestPeerOperationsUAPI_Empty(t *testing.T) {
	result := peerOperationsUAPI(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestWriteAllowedIPs(t *testing.T) {
	ips := []net.IPNet{
		mustParseCIDR(t, "10.0.0.1/32"),
		mustParseCIDR(t, "10.0.1.0/24"),
	}
	var sb strings.Builder
	writeAllowedIPs(&sb, ips)
	result := sb.String()

	if strings.Count(result, "allowed_ip=") != 2 {
		t.Errorf("expected 2 allowed_ip lines, got:\n%s", result)
	}
	if !strings.Contains(result, "allowed_ip=10.0.0.1/32") {
		t.Error("missing 10.0.0.1/32")
	}
	if !strings.Contains(result, "allowed_ip=10.0.1.0/24") {
		t.Error("missing 10.0.1.0/24")
	}
}

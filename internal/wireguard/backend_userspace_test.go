package wireguard

import (
	"encoding/hex"
	"net"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestParseUapiStatus_ObservedPeerState(t *testing.T) {
	keyHex := strings.Repeat("01", wgtypes.KeyLen)
	raw := strings.Join([]string{
		"listen_port=51820",
		"public_key=" + keyHex,
		"endpoint=198.51.100.10:51820",
		"last_handshake_time_sec=100",
		"last_handshake_time_nsec=200",
		"persistent_keepalive_interval=25",
		"allowed_ip=10.33.1.1/32",
		"rx_bytes=123",
		"tx_bytes=456",
	}, "\n")

	status, err := parseUapiStatus("utun8", raw)
	if err != nil {
		t.Fatalf("parseUapiStatus: %v", err)
	}
	if status.Name != "utun8" || status.ListenPort != 51820 {
		t.Fatalf("device status = %+v", status)
	}
	if len(status.Peers) != 1 {
		t.Fatalf("peers = %d, want 1", len(status.Peers))
	}

	keyBytes, _ := hex.DecodeString(keyHex)
	peer := status.Peers[0]
	if peer.PublicKey != wgtypes.Key(keyBytes) {
		t.Fatalf("public key = %s", peer.PublicKey.String())
	}
	if peer.Endpoint == nil || peer.Endpoint.String() != "198.51.100.10:51820" {
		t.Fatalf("endpoint = %v", peer.Endpoint)
	}
	if !peer.LastHandshake.Equal(time.Unix(100, 200)) {
		t.Fatalf("last handshake = %v", peer.LastHandshake)
	}
	if peer.PersistentKeepalive != 25*time.Second {
		t.Fatalf("keepalive = %v", peer.PersistentKeepalive)
	}
	if len(peer.AllowedIPs) != 1 || peer.AllowedIPs[0].String() != (&net.IPNet{
		IP:   net.ParseIP("10.33.1.1"),
		Mask: net.CIDRMask(32, 32),
	}).String() {
		t.Fatalf("allowed IPs = %v", peer.AllowedIPs)
	}
	if peer.ReceiveBytes != 123 || peer.TransmitBytes != 456 {
		t.Fatalf("traffic = rx %d tx %d", peer.ReceiveBytes, peer.TransmitBytes)
	}
}

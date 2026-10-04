package runtime_test

import (
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestGetPeerStatus_NotRunning(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "testnet")
	testutil.SeedPeers(t, env.Database, "testnet", service.Peer{
		Name:      "alice",
		PublicKey: mustGenKey(t),
		Route:     "10.42.0.9/32",
	})

	statuses, err := env.Runtime.GetPeerStatus(t.Context(), "testnet")
	if err != nil {
		t.Fatalf("peer status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("peer statuses = %d, want 1", len(statuses))
	}
	got := statuses[0]
	if got.Name != "alice" || got.Route != "10.42.0.9/32" {
		t.Fatalf("status = %+v, want the cached peer", got)
	}
	if got.Connected || got.Endpoint != "" || !got.LastHandshake.IsZero() {
		t.Fatalf("status = %+v, want zero-valued device fields", got)
	}
}

func TestGetPeerStatus_JoinsLiveDeviceState(
	t *testing.T,
) {
	tests := []struct {
		name          string
		lastHandshake time.Time
		wantConnected bool
	}{
		{
			name:          "fresh handshake",
			lastHandshake: testutil.FixedTime,
			wantConnected: true,
		},
		{
			name:          "stale handshake",
			lastHandshake: testutil.FixedTime.Add(-2 * runtime.StaleThreshold),
			wantConnected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.SetupRuntime(t)
			testutil.SeedNetworkDirect(t, env.Database, "testnet")

			peerKey := mustGenKey(t)
			testutil.SeedPeers(t, env.Database, "testnet", service.Peer{
				Name:      "alice",
				PublicKey: peerKey,
				Route:     "10.42.0.9/32",
			})
			env.Enable(t, "testnet")

			key, err := wgtypes.ParseKey(peerKey)
			if err != nil {
				t.Fatalf("parse key: %v", err)
			}
			env.Backend.Device("testnet").SetPeers(wireguard.PeerStatus{
				PublicKey:     key,
				Endpoint:      &net.UDPAddr{IP: net.ParseIP("5.6.7.8"), Port: 51820},
				LastHandshake: tt.lastHandshake,
			})

			statuses, err := env.Runtime.GetPeerStatus(t.Context(), "testnet")
			if err != nil {
				t.Fatalf("peer status: %v", err)
			}
			if len(statuses) != 1 {
				t.Fatalf("peer statuses = %d, want 1", len(statuses))
			}
			got := statuses[0]
			if got.Endpoint != "5.6.7.8:51820" {
				t.Errorf("endpoint = %q, want 5.6.7.8:51820", got.Endpoint)
			}
			if !got.LastHandshake.Equal(tt.lastHandshake) {
				t.Errorf("last handshake = %v, want %v", got.LastHandshake, tt.lastHandshake)
			}
			if got.Connected != tt.wantConnected {
				t.Errorf("connected = %t, want %t", got.Connected, tt.wantConnected)
			}
		})
	}
}

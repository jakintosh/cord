package runtime_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestGetNetworkTopology_JoinsLiveConnectivity(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	key, err := wgtypes.ParseKey(mustGenKey(t))
	if err != nil {
		t.Fatal(err)
	}
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", key.String(), false, true, true)
	env.Enable(t, "testnet")
	for _, tc := range []struct {
		name      string
		handshake time.Time
		connected bool
	}{
		{"recent", testutil.FixedTime.Add(-time.Second), true},
		{"stale", testutil.FixedTime.Add(-wireguard.ActiveHandshakeThreshold), false},
		{"never", time.Time{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env.Backend.Device("testnet").SetPeers(wireguard.PeerStatus{PublicKey: key, LastHandshake: tc.handshake})
			view, err := env.Runtime.GetNetworkTopology(t.Context(), "testnet")
			if err != nil {
				t.Fatal(err)
			}
			if got, exists := view.Connected["alice"]; !exists || got != tc.connected {
				t.Fatalf("alice connectivity = %t, present=%t, want %t", got, exists, tc.connected)
			}
		})
	}
	if _, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", false); err != nil {
		t.Fatal(err)
	}
	view, err := env.Runtime.GetNetworkTopology(t.Context(), "testnet")
	if err != nil {
		t.Fatal(err)
	}
	if got, exists := view.Connected["alice"]; !exists || got {
		t.Fatalf("stopped network connectivity = %+v", view.Connected)
	}
}

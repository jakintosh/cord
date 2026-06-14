package server

import (
	"testing"

	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

func TestDisablePeriodicPeerSync(t *testing.T) {
	runtime := &Runtime{}

	runtime.DisablePeriodicPeerSync()

	if !runtime.disablePeriodicPeerSync {
		t.Fatal("periodic peer sync still enabled")
	}
}

func TestMainPeersIncludesRedeemedUnconfirmedPeer(t *testing.T) {
	serverKey, err := wg.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}
	confirmedKey, err := wg.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate confirmed peer key: %v", err)
	}
	unconfirmedKey, err := wg.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate unconfirmed peer key: %v", err)
	}
	disabledKey, err := wg.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate disabled peer key: %v", err)
	}

	peers := []*Peer{
		{
			Name:      "cord-server",
			PublicKey: serverKey.PublicKey().String(),
			Cidr:      "10.0.0.1/32",
			Enabled:   true,
			Confirmed: true,
		},
		{
			Name:      "confirmed",
			PublicKey: confirmedKey.PublicKey().String(),
			Cidr:      "10.0.0.2/32",
			Enabled:   true,
			Confirmed: true,
		},
		{
			Name:      "redeemed",
			PublicKey: unconfirmedKey.PublicKey().String(),
			Cidr:      "10.0.0.3/32",
			Enabled:   true,
			Confirmed: false,
		},
		{
			Name:      "disabled",
			PublicKey: disabledKey.PublicKey().String(),
			Cidr:      "10.0.0.4/32",
			Enabled:   false,
			Confirmed: false,
		},
	}

	got := mainPeersFromRecords(peers, serverKey.PublicKey().String())
	if len(got) != 2 {
		t.Fatalf("main peers = %d, want confirmed and redeemed peers", len(got))
	}

	keys := map[string]bool{}
	for _, peer := range got {
		keys[peer.PublicKey.String()] = true
	}
	if !keys[confirmedKey.PublicKey().String()] {
		t.Error("confirmed peer missing from main interface")
	}
	if !keys[unconfirmedKey.PublicKey().String()] {
		t.Error("redeemed unconfirmed peer missing from main interface")
	}
}

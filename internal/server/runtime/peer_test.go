package runtime_test

import (
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestPeerCommands_ApplyPeersBeforeReturning(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	if status, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", true); err != nil || status.Health != "healthy" {
		t.Fatalf("enable = %+v, %v", status, err)
	}
	main, invite := env.Backend.Device("testnet"), env.Backend.Device("testnet-i")
	invitation, err := env.Runtime.CreateRegistration(t.Context(), "testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatal(err)
	}
	tempKey, err := wireguard.PublicKey(invitation.Peer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := wireguard.PublicKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Runtime.RedeemRegistration(t.Context(), "testnet", tempKey, publicKey); err != nil {
		t.Fatal(err)
	}
	if err := env.Runtime.ConfirmPeer(t.Context(), "testnet", "alice"); err != nil {
		t.Fatal(err)
	}
	if hasDevicePeer(t, invite, tempKey) {
		t.Fatal("confirmation returned before retiring the invite peer")
	}
	if err := env.Runtime.RemovePeer(t.Context(), "testnet", "alice"); err != nil {
		t.Fatal(err)
	}
	if hasDevicePeer(t, main, publicKey) {
		t.Fatal("removal returned before removing the permanent peer")
	}
	if env.Backend.Device("testnet") != main || env.Backend.Device("testnet-i") != invite || len(env.Backend.CreateCalls) != 2 {
		t.Fatal("peer commands rebuilt the network devices")
	}
}

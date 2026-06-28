package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestCreateInvite_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	invite, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name:      "new-peer",
		IP:        net.ParseIP("10.0.0.5"),
		Admin:     false,
		ExpiresIn: time.Hour,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	if invite.Interface.NetworkName != "testnet" {
		t.Errorf("network_name = %q, want testnet", invite.Interface.NetworkName)
	}
	if invite.Interface.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if invite.Interface.AssignedCidr == "" {
		t.Error("assigned_cidr should not be empty")
	}
	if invite.Server.PublicKey == "" {
		t.Error("server public_key should not be empty")
	}
	if invite.Server.ExternalEndpoint != "192.168.1.1:51821" {
		t.Errorf("external_endpoint = %q, want 192.168.1.1:51821", invite.Server.ExternalEndpoint)
	}
}

func TestCreateInvite_DefaultExpiration(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "default-exp",
		IP:   net.ParseIP("10.0.0.6"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	invites, err := env.Service.ListInvites("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(invites))
	}

	expectedExpiry := testutil.FixedTime.Add(24 * time.Hour)
	if !invites[0].ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("expires_at = %v, want %v", invites[0].ExpiresAt, expectedExpiry)
	}
}

func TestCreateInvite_AutoAssignsIP(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	invite, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "auto-ip",
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	// First auto-assigned IP should be 10.0.0.2 (10.0.0.0 = network, 10.0.0.1 = server)
	if invite.Interface.AssignedCidr != "10.1.0.2/24" {
		t.Errorf("assigned_cidr = %q, want 10.1.0.2/24", invite.Interface.AssignedCidr)
	}
}

func TestCreateInvite_ReconcilesRunningInviteDeviceWithHostRoute(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	invite, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "live-invite",
		IP:   net.ParseIP("10.0.0.5"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	inviteDev := env.WireGuard.Devices["testnet-i"]
	if inviteDev == nil {
		t.Fatal("expected invite device")
	}
	peers := inviteDev.AppliedPeers()
	if len(peers) != 1 {
		t.Fatalf("invite peers = %d, want 1", len(peers))
	}
	if peers[0].PublicKey != invite.Interface.PrivateKey+"-pub" {
		t.Fatalf("public key = %q, want temp invite public key", peers[0].PublicKey)
	}
	if got := peers[0].AllowedIPs; len(got) != 1 || got[0] != "10.1.0.2/32" {
		t.Fatalf("allowed IPs = %v, want [10.1.0.2/32]", got)
	}
}

func TestCreateInvite_EmptyName(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		IP: net.ParseIP("10.0.0.5"),
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateInvite_NonexistentNetwork(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateInvite("nonexistent", service.CreateInviteRequest{
		Name: "peer",
		IP:   net.ParseIP("10.0.0.5"),
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRedeemInvite_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "redeemer",
		IP:   net.ParseIP("10.0.0.5"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	result, err := env.Service.RedeemInvite("testnet", tempKey, "perm-key-1")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if result.NetworkName != "testnet" {
		t.Errorf("network_name = %q, want testnet", result.NetworkName)
	}
	if result.AssignedCidr != "10.0.0.5/16" {
		t.Errorf("assigned_cidr = %q, want 10.0.0.5/16", result.AssignedCidr)
	}
	if result.Server.PublicKey == "" {
		t.Error("server public_key should not be empty")
	}

	peer, err := env.Service.GetPeer("testnet", "redeemer")
	if err != nil {
		t.Fatalf("get redeemed peer: %v", err)
	}
	if peer.PublicKey != "perm-key-1" {
		t.Errorf("public_key = %q, want perm-key-1", peer.PublicKey)
	}
	if peer.Cidr != "10.0.0.5/32" {
		t.Errorf("cidr = %q, want 10.0.0.5/32", peer.Cidr)
	}
}

func TestRedeemInvite_Idempotent_SameKey(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "idempotent",
		IP:   net.ParseIP("10.0.0.6"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	result1, err := env.Service.RedeemInvite("testnet", tempKey, "perm-key-2")
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	result2, err := env.Service.RedeemInvite("testnet", tempKey, "perm-key-2")
	if err != nil {
		t.Fatalf("second redeem: %v", err)
	}

	if result1.AssignedCidr != result2.AssignedCidr {
		t.Errorf("results differ: %q vs %q", result1.AssignedCidr, result2.AssignedCidr)
	}
}

func TestRedeemInvite_UnknownKey(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "peer",
		IP:   net.ParseIP("10.0.0.5"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	_, err = env.Service.RedeemInvite("testnet", "unknown-temp-key", "perm-key")
	if err == nil {
		t.Fatal("expected error for unknown temp key")
	}
}

func TestRedeemInvite_MultipleInvites(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "peer-a",
		IP:   net.ParseIP("10.0.0.10"),
	})
	if err != nil {
		t.Fatalf("create invite a: %v", err)
	}
	tempKey1 := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "peer-b",
		IP:   net.ParseIP("10.0.0.11"),
	})
	if err != nil {
		t.Fatalf("create invite b: %v", err)
	}
	tempKey2 := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemInvite("testnet", tempKey1, "key-a")
	if err != nil {
		t.Fatalf("redeem a: %v", err)
	}

	_, err = env.Service.RedeemInvite("testnet", tempKey2, "key-b")
	if err != nil {
		t.Fatalf("redeem b: %v", err)
	}

	peers, err := env.Service.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers (cord-server + peer-a + peer-b), got %d", len(peers))
	}
}

func TestRedeemInvite_ReconcilesOnlyOnConfirm(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.StartNetwork(context.Background(), "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}
	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "live-redeem",
		IP:   net.ParseIP("10.0.0.5"),
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemInvite("testnet", tempKey, "perm-live-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// After redeem but before confirm, the peer should still be on the
	// invite device (so the client can retry if the response was lost)
	// and NOT yet on the main device.
	inviteDev := env.WireGuard.Devices["testnet-i"]
	if inviteDev == nil {
		t.Fatal("expected invite device")
	}
	if peers := inviteDev.AppliedPeers(); len(peers) != 1 {
		t.Fatalf("invite peers after redeem = %d, want 1 (temp peer still active)", len(peers))
	}

	mainDev := env.WireGuard.Devices["testnet"]
	if mainDev == nil {
		t.Fatal("expected main device")
	}
	for _, peer := range mainDev.AppliedPeers() {
		if peer.PublicKey == "perm-live-key" {
			t.Fatal("main device should not have redeemed peer before confirm")
		}
	}

	// Now confirm — this is what triggers reconciliation.
	if err := env.Service.ConfirmPeer("testnet", "live-redeem"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if peers := inviteDev.AppliedPeers(); len(peers) != 0 {
		t.Fatalf("invite peers after confirm = %d, want 0", len(peers))
	}

	var found bool
	for _, peer := range mainDev.AppliedPeers() {
		if peer.PublicKey == "perm-live-key" {
			found = true
			if got := peer.AllowedIPs; len(got) != 1 || got[0] != "10.0.0.5/32" {
				t.Fatalf("redeemed peer allowed IPs = %v, want [10.0.0.5/32]", got)
			}
		}
	}
	if !found {
		t.Fatal("main device missing redeemed peer after confirm")
	}
}

func TestListInvites_Mixed(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "active",
		IP:   net.ParseIP("10.0.0.20"),
	})
	if err != nil {
		t.Fatalf("create active: %v", err)
	}

	_, err = env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "to-redeem",
		IP:   net.ParseIP("10.0.0.21"),
	})
	if err != nil {
		t.Fatalf("create to-redeem: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemInvite("testnet", tempKey, "redeemed-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	invites, err := env.Service.ListInvites("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invites, got %d", len(invites))
	}

	var redeemed, active int
	for _, inv := range invites {
		if inv.Redeemed {
			redeemed++
		} else {
			active++
		}
	}
	if redeemed != 1 {
		t.Errorf("expected 1 redeemed, got %d", redeemed)
	}
	if active != 1 {
		t.Errorf("expected 1 active, got %d", active)
	}
}

func TestRevokeInvite_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name: "revoke-me",
		IP:   net.ParseIP("10.0.0.30"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := env.Service.RevokeInvite("testnet", "revoke-me"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	invites, err := env.Service.ListInvites("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("expected 0 invites after revoke, got %d", len(invites))
	}
}

func TestRevokeInvite_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.RevokeInvite("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestParseInvite_Success(t *testing.T) {
	input := `{
		"interface": {
			"network_name": "testnet",
			"private_key": "temp-key",
			"assigned_cidr": "10.1.0.2/24"
		},
		"server": {
			"public_key": "server-key",
			"external_endpoint": "1.2.3.4:51820",
			"internal_endpoint": "10.0.0.1:8080"
		}
	}`

	inv, err := service.ParseInvite(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if inv.Interface.NetworkName != "testnet" {
		t.Errorf("network_name = %q, want testnet", inv.Interface.NetworkName)
	}
	if inv.Interface.PrivateKey != "temp-key" {
		t.Errorf("private_key = %q, want temp-key", inv.Interface.PrivateKey)
	}
	if inv.Interface.AssignedCidr != "10.1.0.2/24" {
		t.Errorf("assigned_cidr = %q, want 10.1.0.2/24", inv.Interface.AssignedCidr)
	}
	if inv.Server.PublicKey != "server-key" {
		t.Errorf("server public_key = %q, want server-key", inv.Server.PublicKey)
	}
	if inv.Server.ExternalEndpoint != "1.2.3.4:51820" {
		t.Errorf("external_endpoint = %q, want 1.2.3.4:51820", inv.Server.ExternalEndpoint)
	}
	if inv.Server.InternalEndpoint != "10.0.0.1:8080" {
		t.Errorf("internal_endpoint = %q, want 10.0.0.1:8080", inv.Server.InternalEndpoint)
	}
}

func TestParseInvite_InvalidJSON(t *testing.T) {
	input := `{not valid json`

	_, err := service.ParseInvite(bytes.NewReader([]byte(input)))
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestPeerInvite_Write(t *testing.T) {
	inv := &service.PeerInvite{
		Interface: service.InviteInterface{
			NetworkName:  "mynet",
			PrivateKey:   "priv",
			AssignedCidr: "10.0.0.5/24",
		},
		Server: service.ServerInfo{
			PublicKey:        "pub",
			ExternalEndpoint: "1.2.3.4:51820",
			InternalEndpoint: "10.0.0.1:8080",
		},
	}

	var buf bytes.Buffer
	if err := inv.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	var parsed service.PeerInvite
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal written: %v", err)
	}

	if parsed.Interface.NetworkName != "mynet" {
		t.Errorf("network_name = %q, want mynet", parsed.Interface.NetworkName)
	}
	if parsed.Server.PublicKey != "pub" {
		t.Errorf("public_key = %q, want pub", parsed.Server.PublicKey)
	}
}

func TestInvite_Persistence(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateInvite("testnet", service.CreateInviteRequest{
		Name:      "persist-test",
		IP:        net.ParseIP("10.0.0.5"),
		Admin:     true,
		ExpiresIn: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	invites, err := env.Service.ListInvites("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(invites))
	}

	inv := invites[0]
	if inv.Name != "persist-test" {
		t.Errorf("name = %q, want persist-test", inv.Name)
	}
	if inv.TempPubKey == "" {
		t.Error("temp_pub_key should not be empty")
	}
	if !inv.Admin {
		t.Error("admin should be true")
	}
	if inv.Redeemed {
		t.Error("should not be redeemed")
	}
	if inv.RedeemedKey != "" {
		t.Errorf("redeemed_key = %q, want empty", inv.RedeemedKey)
	}
	if inv.FinalIP == nil || inv.FinalIP.String() != "10.0.0.5" {
		t.Errorf("final_ip = %v, want 10.0.0.5", inv.FinalIP)
	}
	if inv.TempIP == nil {
		t.Error("temp_ip should not be nil")
	}
	if inv.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
	if inv.ExpiresAt.IsZero() {
		t.Error("expires_at should not be zero")
	}
}

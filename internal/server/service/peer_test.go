package service_test

import (
	"errors"
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func TestAddPeer_AutoAssignsIP(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	invite, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name:  "alice",
		Admin: false,
	})
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}

	if invite == nil {
		t.Fatal("expected invite, got nil")
	}
	if invite.Interface.NetworkName != "testnet" {
		t.Errorf("network_name = %q, want testnet", invite.Interface.NetworkName)
	}
	if invite.Interface.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	// First auto-assigned IP should be 10.1.0.2 (10.1.0.0 = network, 10.1.0.1 = server)
	if invite.Interface.AssignedCidr != "10.1.0.2/24" {
		t.Errorf("assigned_cidr = %q, want 10.1.0.2/24", invite.Interface.AssignedCidr)
	}
}

func TestAddPeer_SpecifiedIP(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	invite, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name:  "bob",
		IP:    "10.0.5.10",
		Admin: true,
	})
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}

	if invite == nil {
		t.Fatal("expected invite, got nil")
	}
	if invite.Server.PublicKey == "" {
		t.Error("server public_key should not be empty")
	}
}

func TestAddPeer_InvalidIP(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "badip",
		IP:   "not-an-ip",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddPeer_IPOutsideRootCIDR(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "outside",
		IP:   "192.168.1.1",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestAddPeer_EmptyName(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		IP: "10.0.0.5",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestGetPeer_ViaRedeem(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name:  "carol",
		IP:    "10.0.0.50",
		Admin: false,
	})
	if err != nil {
		t.Fatalf("add peer: %v", err)
	}

	tempKey := lastTempKey(t, env.svc, "testnet")
	result, err := env.svc.RedeemInvite("testnet", tempKey, "carol-perm-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if result.NetworkName != "testnet" {
		t.Errorf("result network = %q, want testnet", result.NetworkName)
	}

	peer, err := env.svc.GetPeer("testnet", "carol")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.Name != "carol" {
		t.Errorf("name = %q, want carol", peer.Name)
	}
	if peer.PublicKey != "carol-perm-key" {
		t.Errorf("public_key = %q, want carol-perm-key", peer.PublicKey)
	}
	if peer.Cidr != "10.0.0.50/32" {
		t.Errorf("cidr = %q, want 10.0.0.50/32", peer.Cidr)
	}
}

func TestGetPeer_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.GetPeer("testnet", "nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListPeers_Empty(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	peers, err := env.svc.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer (cord-server), got %d", len(peers))
	}
}

func TestListPeers_AfterRedeem(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "peer1",
		IP:   "10.0.0.10",
	})
	if err != nil {
		t.Fatalf("add peer1: %v", err)
	}
	tempKey1 := lastTempKey(t, env.svc, "testnet")

	_, err = env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "peer2",
		IP:   "10.0.0.11",
	})
	if err != nil {
		t.Fatalf("add peer2: %v", err)
	}
	tempKey2 := lastTempKey(t, env.svc, "testnet")

	_, err = env.svc.RedeemInvite("testnet", tempKey1, "key1")
	if err != nil {
		t.Fatalf("redeem peer1: %v", err)
	}

	peers, err := env.svc.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers (cord-server + peer1), got %d", len(peers))
	}
	if peers[0].Name != "cord-server" {
		t.Errorf("name = %q, want cord-server", peers[0].Name)
	}
	if peers[1].Name != "peer1" {
		t.Errorf("name = %q, want peer1", peers[1].Name)
	}

	_ = tempKey2
}

func TestRemovePeer_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "removeme",
		IP:   "10.0.0.20",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "removeme-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.svc.RemovePeer("testnet", "removeme"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err = env.svc.GetPeer("testnet", "removeme")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after remove: err = %v, want ErrNotFound", err)
	}
}

func TestRemovePeer_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.RemovePeer("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdatePeer_Rename(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "old-name",
		IP:   "10.0.0.30",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "rename-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	newName := "new-name"
	peer, err := env.svc.UpdatePeer("testnet", "old-name", service.UpdatePeerRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if peer.Name != "new-name" {
		t.Errorf("name = %q, want new-name", peer.Name)
	}
}

func TestUpdatePeer_ToggleAdmin(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name:  "toggle",
		IP:    "10.0.0.31",
		Admin: false,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "toggle-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	adminTrue := true
	peer, err := env.svc.UpdatePeer("testnet", "toggle", service.UpdatePeerRequest{
		Admin: &adminTrue,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !peer.Admin {
		t.Error("peer should be admin")
	}
}

func TestUpdatePeer_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	newName := "x"
	_, err := env.svc.UpdatePeer("testnet", "nonexistent", service.UpdatePeerRequest{
		Name: &newName,
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEnablePeer_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "enableme",
		IP:   "10.0.0.40",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "enable-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	_, err = env.svc.UpdatePeer("testnet", "enableme", service.UpdatePeerRequest{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}

	if err := env.svc.EnablePeer("testnet", "enableme"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	peer, err := env.svc.GetPeer("testnet", "enableme")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !peer.Enabled {
		t.Error("peer should be enabled")
	}
}

func TestDisablePeer_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "disableme",
		IP:   "10.0.0.41",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "disable-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.svc.DisablePeer("testnet", "disableme"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	peer, err := env.svc.GetPeer("testnet", "disableme")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if peer.Enabled {
		t.Error("peer should be disabled")
	}
}

func TestConfirmPeer_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "confirmme",
		IP:   "10.0.0.50",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "confirm-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	peer, err := env.svc.GetPeer("testnet", "confirmme")
	if err != nil {
		t.Fatalf("get before confirm: %v", err)
	}
	if peer.Confirmed {
		t.Error("peer should not be confirmed yet")
	}

	err = env.svc.ConfirmPeer("testnet", "confirm-key", net.IPv4(10, 0, 0, 50))
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	peer, err = env.svc.GetPeer("testnet", "confirmme")
	if err != nil {
		t.Fatalf("get after confirm: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should be confirmed")
	}
}

func TestConfirmPeer_IPMismatch(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "mismatch",
		IP:   "10.0.0.60",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	tempKey := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey, "mismatch-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	err = env.svc.ConfirmPeer("testnet", "mismatch-key", net.IPv4(10, 0, 0, 99))
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestConfirmPeer_UnknownKey(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	err := env.svc.ConfirmPeer("testnet", "unknown-key", net.IPv4(10, 0, 0, 1))
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListVisiblePeers_ExcludesSelf(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	_, err := env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "self",
		IP:   "10.0.0.10",
	})
	if err != nil {
		t.Fatalf("add self: %v", err)
	}
	tempKey1 := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey1, "self-key")
	if err != nil {
		t.Fatalf("redeem self: %v", err)
	}

	_, err = env.svc.AddPeer("testnet", service.PeerConfig{
		Name: "other",
		IP:   "10.0.0.11",
	})
	if err != nil {
		t.Fatalf("add other: %v", err)
	}
	tempKey2 := lastTempKey(t, env.svc, "testnet")
	_, err = env.svc.RedeemInvite("testnet", tempKey2, "other-key")
	if err != nil {
		t.Fatalf("redeem other: %v", err)
	}

	visible, err := env.svc.ListVisiblePeers("testnet", "self")
	if err != nil {
		t.Fatalf("list visible: %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("expected 2 visible peers (cord-server + other), got %d", len(visible))
	}
	foundOther := false
	for _, p := range visible {
		if p.Name == "other" {
			foundOther = true
			break
		}
	}
	if !foundOther {
		t.Errorf("expected 'other' in visible peers, got %+v", visible)
	}
}

func TestReportEndpoints_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc)

	sightings := []service.EndpointSighting{
		{
			WitnessKey: "witness-key",
			PeerKey:    "peer-key",
			Endpoint:   "1.2.3.4:51820",
			Timestamp:  fixedTime,
		},
	}

	err := env.svc.ReportEndpoints("testnet", sightings)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
}

func TestReportEndpoints_NonexistentNetwork(t *testing.T) {
	env := setupTestEnv(t)

	err := env.svc.ReportEndpoints("nonexistent", nil)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

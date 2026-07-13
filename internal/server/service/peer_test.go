package service_test

import (
	"errors"
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestGetPeer_ViaRedeem(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.50")
	_, err := env.Service.CreateRegistration("testnet", "carol", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	result, err := env.Service.RedeemRegistration("testnet", tempKey, "carol-perm-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if result.Network.Name != "testnet" {
		t.Errorf("result network = %q, want testnet", result.Network.Name)
	}

	peer, err := env.Service.GetPeer("testnet", "carol")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.Name != "carol" {
		t.Errorf("name = %q, want carol", peer.Name)
	}
	if peer.PublicKey != "carol-perm-key" {
		t.Errorf("public_key = %q, want carol-perm-key", peer.PublicKey)
	}
	if peer.Route != "10.0.0.50/32" {
		t.Errorf("route = %q, want 10.0.0.50/32", peer.Route)
	}
}

func TestGetPeer_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.GetPeer("testnet", "nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListPeers_Empty(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	peers, err := env.Service.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer (cord-server), got %d", len(peers))
	}
}

func TestListPeers_AfterRedeem(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip1 := net.ParseIP("10.0.0.10")
	_, err := env.Service.CreateRegistration("testnet", "peer1", service.RegistrationOptions{IP: ip1})
	if err != nil {
		t.Fatalf("create peer1: %v", err)
	}
	tempKey1 := lastTempKey(t, env.Service, "testnet")

	ip2 := net.ParseIP("10.0.0.11")
	_, err = env.Service.CreateRegistration("testnet", "peer2", service.RegistrationOptions{IP: ip2})
	if err != nil {
		t.Fatalf("create peer2: %v", err)
	}
	tempKey2 := lastTempKey(t, env.Service, "testnet")

	_, err = env.Service.RedeemRegistration("testnet", tempKey1, "key1")
	if err != nil {
		t.Fatalf("redeem peer1: %v", err)
	}

	peers, err := env.Service.ListPeers("testnet")
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
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.20")
	_, err := env.Service.CreateRegistration("testnet", "removeme", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "removeme-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if err := env.Service.RemovePeer("testnet", "removeme"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, err = env.Service.GetPeer("testnet", "removeme")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after remove: err = %v, want ErrNotFound", err)
	}
}

func TestRemovePeer_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.RemovePeer("testnet", "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdatePeer_Rename(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.30")
	_, err := env.Service.CreateRegistration("testnet", "old-name", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "rename-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	newName := "new-name"
	peer, err := env.Service.UpdatePeer("testnet", "old-name", service.PeerUpdate{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if peer.Name != "new-name" {
		t.Errorf("name = %q, want new-name", peer.Name)
	}
}

func TestUpdatePeer_ToggleAdmin(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.31")
	_, err := env.Service.CreateRegistration("testnet", "toggle", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "toggle-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	adminTrue := true
	peer, err := env.Service.UpdatePeer("testnet", "toggle", service.PeerUpdate{Admin: &adminTrue})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !peer.Admin {
		t.Error("peer should be admin")
	}
}

func TestUpdatePeer_NotFound(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	newName := "x"
	_, err := env.Service.UpdatePeer("testnet", "nonexistent", service.PeerUpdate{Name: &newName})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEnablePeer_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.40")
	_, err := env.Service.CreateRegistration("testnet", "enableme", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "enable-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	_, err = env.Service.UpdatePeer("testnet", "enableme", service.PeerUpdate{Enabled: &disabled})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}

	enabled := true
	if _, err := env.Service.UpdatePeer("testnet", "enableme", service.PeerUpdate{Enabled: &enabled}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	peer, err := env.Service.GetPeer("testnet", "enableme")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !peer.Enabled {
		t.Error("peer should be enabled")
	}
}

func TestDisablePeer_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.41")
	_, err := env.Service.CreateRegistration("testnet", "disableme", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "disable-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	if _, err := env.Service.UpdatePeer("testnet", "disableme", service.PeerUpdate{Enabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	peer, err := env.Service.GetPeer("testnet", "disableme")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if peer.Enabled {
		t.Error("peer should be disabled")
	}
}

func TestConfirmPeer_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.50")
	_, err := env.Service.CreateRegistration("testnet", "confirmme", service.RegistrationOptions{IP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}

	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "confirm-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	peer, err := env.Service.GetPeer("testnet", "confirmme")
	if err != nil {
		t.Fatalf("get before confirm: %v", err)
	}
	if peer.Confirmed {
		t.Error("peer should not be confirmed yet")
	}

	err = env.Service.ConfirmPeer("testnet", "confirmme")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	peer, err = env.Service.GetPeer("testnet", "confirmme")
	if err != nil {
		t.Fatalf("get after confirm: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should be confirmed")
	}
}

func TestConfirmPeer_UnknownPeer(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	err := env.Service.ConfirmPeer("testnet", "nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListVisiblePeers_ExcludesSelf(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip1 := net.ParseIP("10.0.0.10")
	_, err := env.Service.CreateRegistration("testnet", "self", service.RegistrationOptions{IP: ip1})
	if err != nil {
		t.Fatalf("create self: %v", err)
	}
	tempKey1 := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey1, "self-key")
	if err != nil {
		t.Fatalf("redeem self: %v", err)
	}

	ip2 := net.ParseIP("10.0.0.11")
	_, err = env.Service.CreateRegistration("testnet", "other", service.RegistrationOptions{IP: ip2})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	tempKey2 := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey2, "other-key")
	if err != nil {
		t.Fatalf("redeem other: %v", err)
	}

	visible, err := env.Service.ListVisiblePeers("testnet", "self")
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
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	sightings := []service.EndpointSighting{
		{
			WitnessKey: "witness-key",
			PeerKey:    "peer-key",
			Endpoint:   "1.2.3.4:51820",
			Timestamp:  testutil.FixedTime,
		},
	}

	err := env.Service.ReportEndpoints("testnet", sightings)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
}

func TestReportEndpoints_NonexistentNetwork(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.ReportEndpoints("nonexistent", nil)
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

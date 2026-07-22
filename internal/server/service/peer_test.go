package service_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestGetPeer_ViaRedeem(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.50")
	_, err := env.Service.CreateRegistration("testnet", "carol", service.RegistrationOptions{PeerIP: ip})
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
	_, err := env.Service.CreateRegistration("testnet", "peer1", service.RegistrationOptions{PeerIP: ip1})
	if err != nil {
		t.Fatalf("create peer1: %v", err)
	}
	tempKey1 := lastTempKey(t, env.Service, "testnet")

	ip2 := net.ParseIP("10.0.0.11")
	_, err = env.Service.CreateRegistration("testnet", "peer2", service.RegistrationOptions{PeerIP: ip2})
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
	_, err := env.Service.CreateRegistration("testnet", "removeme", service.RegistrationOptions{PeerIP: ip})
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

func TestRemovePeer_ReleasesRegistrationAndCIDR(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("create group: %v", err)
	}
	ip := net.ParseIP("10.0.0.30")
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "alice", "engineering"); err != nil {
		t.Fatalf("assign registration group: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := env.Service.RemovePeer("testnet", "alice"); err != nil {
		t.Fatalf("remove peer: %v", err)
	}

	if _, err := env.Service.GetPeer("testnet", "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("peer after removal: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Database.GetCidr("testnet", "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("CIDR after removal: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Database.GetRegistration("testnet", "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("registration after removal: err = %v, want ErrNotFound", err)
	}
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("reuse removed peer name and IP: %v", err)
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
	_, err := env.Service.CreateRegistration("testnet", "old-name", service.RegistrationOptions{PeerIP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "rename-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	newName := "new-name"
	peer, err := env.Service.UpdatePeer("testnet", "old-name", service.PeerDiff{Name: &newName})
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
	_, err := env.Service.CreateRegistration("testnet", "toggle", service.RegistrationOptions{PeerIP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "toggle-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	adminTrue := true
	peer, err := env.Service.UpdatePeer("testnet", "toggle", service.PeerDiff{Admin: &adminTrue})
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
	_, err := env.Service.UpdatePeer("testnet", "nonexistent", service.PeerDiff{Name: &newName})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEnablePeer_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.40")
	_, err := env.Service.CreateRegistration("testnet", "enableme", service.RegistrationOptions{PeerIP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "enable-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	_, err = env.Service.UpdatePeer("testnet", "enableme", service.PeerDiff{Enabled: &disabled})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}

	enabled := true
	if _, err := env.Service.UpdatePeer("testnet", "enableme", service.PeerDiff{Enabled: &enabled}); err != nil {
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
	_, err := env.Service.CreateRegistration("testnet", "disableme", service.RegistrationOptions{PeerIP: ip})
	if err != nil {
		t.Fatalf("create reg: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey, "disable-key")
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}

	disabled := false
	if _, err := env.Service.UpdatePeer("testnet", "disableme", service.PeerDiff{Enabled: &disabled}); err != nil {
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
	_, err := env.Service.CreateRegistration("testnet", "confirmme", service.RegistrationOptions{PeerIP: ip})
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
	if err := env.Service.ConfirmPeer("testnet", "confirmme"); err != nil {
		t.Fatalf("confirm idempotently: %v", err)
	}

	peer, err = env.Service.GetPeer("testnet", "confirmme")
	if err != nil {
		t.Fatalf("get after confirm: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should be confirmed")
	}
}

func TestConfirmPeer_PreservesDisabledState(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.CreateRegistration("testnet", "bob", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.6")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	disabled := false
	if _, err := env.Service.UpdatePeer("testnet", "bob", service.PeerDiff{Enabled: &disabled}); err != nil {
		t.Fatalf("disable peer: %v", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "bob"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	peer, err := env.Service.GetPeer("testnet", "bob")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !peer.Confirmed {
		t.Error("peer should be confirmed")
	}
	if peer.Enabled {
		t.Error("peer should still be disabled after confirm")
	}
}

func TestConfirmPeer_ReconcilesRunningDevices(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.EnableNetwork("testnet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	tempKey := lastTempKey(t, env.Service, "testnet")
	permKey := mustGenKey(t)
	if _, err := env.Service.RedeemRegistration("testnet", tempKey, permKey); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if hasPeerOp(env.Backend.LastAppliedOpsFor("testnet-i"), tempKey) {
		t.Error("invite device should not have confirmed registration peer")
	}
	if !hasPeerOp(env.Backend.LastAppliedOpsFor("testnet"), permKey) {
		t.Error("main device missing confirmed peer")
	}
}

func TestConfirmPeer_FollowsPermanentKeyAfterProvisionalRename(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := env.Service.CreateRegistration("testnet", "old-name", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.51")}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "old-name", "engineering"); err != nil {
		t.Fatalf("assign registration group: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	newName := "new-name"
	if _, err := env.Service.UpdatePeer("testnet", "old-name", service.PeerDiff{Name: &newName}); err != nil {
		t.Fatalf("rename provisional peer: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", newName); err != nil {
		t.Fatalf("confirm renamed peer: %v", err)
	}

	reg, err := env.Database.GetRegistration("testnet", "old-name")
	if err != nil {
		t.Fatalf("get registration after confirm: %v", err)
	}
	if !reg.Confirmed {
		t.Fatal("registration should be confirmed")
	}
	groups, err := env.Service.ListCidrGroups("testnet", "old-name")
	if err != nil {
		t.Fatalf("list CIDR groups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "engineering" {
		t.Fatalf("CIDR groups = %+v, want engineering", groups)
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

func TestConfirmPeer_UsesInjectedTimeForExpiration(t *testing.T) {
	clock := &mutableClock{t: testutil.FixedTime}
	env := testutil.SetupServiceWithClock(t, clock.now)
	testutil.SeedNetwork(t, env.Service)
	expiresIn := time.Hour

	if _, err := env.Service.CreateRegistration(
		"testnet",
		"expiring",
		service.RegistrationOptions{
			PeerIP:    net.ParseIP("10.0.0.52"),
			ExpiresIn: &expiresIn,
		},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration(
		"testnet",
		lastTempKey(t, env.Service, "testnet"),
		mustGenKey(t),
	); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}

	clock.t = clock.t.Add(expiresIn)
	if err := env.Service.ConfirmPeer("testnet", "expiring"); !errors.Is(err, service.ErrRegistrationExpired) {
		t.Fatalf("confirm expired peer: err = %v, want ErrRegistrationExpired", err)
	}
	peer, err := env.Service.GetPeer("testnet", "expiring")
	if err != nil {
		t.Fatalf("get peer after rejected confirmation: %v", err)
	}
	if peer.Confirmed {
		t.Fatal("peer should remain unconfirmed")
	}
}

func TestListVisiblePeers_ExcludesSelf(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip1 := net.ParseIP("10.0.0.10")
	_, err := env.Service.CreateRegistration("testnet", "self", service.RegistrationOptions{PeerIP: ip1})
	if err != nil {
		t.Fatalf("create self: %v", err)
	}
	tempKey1 := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey1, "self-key")
	if err != nil {
		t.Fatalf("redeem self: %v", err)
	}

	ip2 := net.ParseIP("10.0.0.11")
	_, err = env.Service.CreateRegistration("testnet", "other", service.RegistrationOptions{PeerIP: ip2})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	tempKey2 := lastTempKey(t, env.Service, "testnet")
	_, err = env.Service.RedeemRegistration("testnet", tempKey2, "other-key")
	if err != nil {
		t.Fatalf("redeem other: %v", err)
	}

	_, err = env.Service.CreateGroup("testnet", "test-group")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "self", "test-group"); err != nil {
		t.Fatalf("assign self: %v", err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "other", "test-group"); err != nil {
		t.Fatalf("assign other: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "self"); err != nil {
		t.Fatalf("confirm self: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "other"); err != nil {
		t.Fatalf("confirm other: %v", err)
	}
	if err := env.Service.CreateAssociation("testnet", "test-group", "test-group"); err != nil {
		t.Fatalf("create association: %v", err)
	}

	visible, err := env.Service.ListVisiblePeers("testnet", "self")
	if err != nil {
		t.Fatalf("list visible: %v", err)
	}
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible peer (other), got %d", len(visible))
	}
	if visible[0].Name != "other" {
		t.Errorf("expected 'other', got %s", visible[0].Name)
	}
}

func TestReportEndpoints_Success(t *testing.T) {
	env := testutil.SetupService(t)
	network := testutil.SeedNetwork(t, env.Service)

	sightings := []service.EndpointSighting{
		{
			WitnessKey: network.PublicKey,
			PeerKey:    network.PublicKey,
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

func TestReportEndpoints_NonexistentPeer(t *testing.T) {
	env := testutil.SetupService(t)
	network := testutil.SeedNetwork(t, env.Service)

	err := env.Service.ReportEndpoints("testnet", []service.EndpointSighting{
		{
			WitnessKey: network.PublicKey,
			PeerKey:    "missing-key",
			Endpoint:   "1.2.3.4:51820",
			Timestamp:  testutil.FixedTime,
		},
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

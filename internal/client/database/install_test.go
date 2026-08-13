package database_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func defaultBeginInstall(
	name string,
) service.BeginInstallParams {
	return service.BeginInstallParams{
		Name:                name,
		ListenPort:          51820,
		InviteIfaceName:     name + "-i",
		InvitePrivateKey:    "invite-priv-key",
		InviteAssignedRoute: "10.43.0.5/32",
		InviteServer: service.ServerInfo{
			PublicKey:   "invite-server-pubkey",
			Endpoint:    "1.2.3.4:51821",
			Route:       "10.43.0.1/32",
			NetworkCidr: "10.43.0.0/24",
			APIPort:     8443,
		},
		MainIfaceName:  name,
		MainPrivateKey: "main-priv-key",
		CreatedAt:      testutil.FixedTime,
	}
}

func defaultNetworkAssignment() service.NetworkAssignment {
	return service.NetworkAssignment{
		AssignedRoute: "10.42.0.5/32",
		Server: service.ServerInfo{
			PublicKey:   "main-server-pubkey",
			Endpoint:    "5.6.7.8:51820",
			Route:       "10.42.0.1/32",
			NetworkCidr: "10.42.0.0/16",
			APIPort:     8443,
		},
	}
}

func beginInstall(
	t *testing.T,
	db interface {
		BeginInstall(service.BeginInstallParams) (*service.Install, error)
	},
	name string,
) *service.Install {
	t.Helper()

	install, err := db.BeginInstall(defaultBeginInstall(name))
	if err != nil {
		t.Fatalf("begin install %q: %v", name, err)
	}
	return install
}

func redeemInstall(
	t *testing.T,
	db interface {
		RedeemInstall(string, service.NetworkAssignment) (*service.Install, error)
	},
	name string,
) *service.Install {
	t.Helper()

	install, err := db.RedeemInstall(name, defaultNetworkAssignment())
	if err != nil {
		t.Fatalf("redeem install %q: %v", name, err)
	}
	return install
}

func TestGetInstall_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.GetInstall("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestBeginInstall_CreatesInvitedInstall(t *testing.T) {
	db := testutil.SetupDB(t)
	params := defaultBeginInstall("testnet")

	install, err := db.BeginInstall(params)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	if install.Name != params.Name {
		t.Errorf("name = %q, want %q", install.Name, params.Name)
	}
	if install.Phase != service.PhaseInvited {
		t.Errorf("phase = %q, want %q", install.Phase, service.PhaseInvited)
	}
	if install.MainPrivateKey != params.MainPrivateKey {
		t.Errorf("main private key = %q, want %q", install.MainPrivateKey, params.MainPrivateKey)
	}
	if !install.CreatedAt.Equal(params.CreatedAt) {
		t.Errorf("created at = %v, want %v", install.CreatedAt, params.CreatedAt)
	}

	persisted, err := db.GetInstall(params.Name)
	if err != nil {
		t.Fatalf("get install: %v", err)
	}
	if persisted.MainPrivateKey != params.MainPrivateKey {
		t.Errorf("persisted main private key = %q, want %q", persisted.MainPrivateKey, params.MainPrivateKey)
	}
}

func TestBeginInstall_CompatibleRetryReturnsExistingInstall(t *testing.T) {
	db := testutil.SetupDB(t)
	params := defaultBeginInstall("testnet")

	first, err := db.BeginInstall(params)
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	retry := params
	retry.MainPrivateKey = "replacement-key-that-must-not-win"
	retry.CreatedAt = params.CreatedAt.Add(time.Hour)
	second, err := db.BeginInstall(retry)
	if err != nil {
		t.Fatalf("retry begin: %v", err)
	}

	if second.MainPrivateKey != first.MainPrivateKey {
		t.Errorf("retry private key = %q, want original %q", second.MainPrivateKey, first.MainPrivateKey)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("retry created at = %v, want original %v", second.CreatedAt, first.CreatedAt)
	}
}

func TestBeginInstall_IncompatibleRetryLeavesExistingInstallUnchanged(t *testing.T) {
	db := testutil.SetupDB(t)
	params := defaultBeginInstall("testnet")
	first, err := db.BeginInstall(params)
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	retry := params
	retry.InviteAssignedRoute = "10.43.0.6/32"
	_, err = db.BeginInstall(retry)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	persisted, err := db.GetInstall(params.Name)
	if err != nil {
		t.Fatalf("get existing install: %v", err)
	}
	if persisted.InviteAssignedRoute != first.InviteAssignedRoute {
		t.Errorf(
			"invite route = %q, want unchanged %q",
			persisted.InviteAssignedRoute,
			first.InviteAssignedRoute,
		)
	}
}

func TestBeginInstall_CompletedNetworkReturnsNetworkExists(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	redeemInstall(t, db, "testnet")
	if _, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime); err != nil {
		t.Fatalf("confirm install: %v", err)
	}

	_, err := db.BeginInstall(defaultBeginInstall("testnet"))
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Fatalf("err = %v, want ErrNetworkExists", err)
	}
}

func TestListInstalls_Ordered(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "beta")
	beginInstall(t, db, "alpha")

	installs, err := db.ListInstalls()
	if err != nil {
		t.Fatalf("list installs: %v", err)
	}
	if len(installs) != 2 {
		t.Fatalf("installs = %d, want 2", len(installs))
	}
	if installs[0].Name != "alpha" || installs[1].Name != "beta" {
		t.Fatalf("unexpected order: %q, %q", installs[0].Name, installs[1].Name)
	}
}

func TestRedeemInstall_TransitionsAndPreservesInviteIdentity(t *testing.T) {
	db := testutil.SetupDB(t)
	invited := beginInstall(t, db, "testnet")
	assignment := defaultNetworkAssignment()

	redeemed, err := db.RedeemInstall("testnet", assignment)
	if err != nil {
		t.Fatalf("redeem install: %v", err)
	}

	if redeemed.Phase != service.PhaseRedeemed {
		t.Errorf("phase = %q, want %q", redeemed.Phase, service.PhaseRedeemed)
	}
	if redeemed.MainAssignedRoute != assignment.AssignedRoute {
		t.Errorf("main route = %q, want %q", redeemed.MainAssignedRoute, assignment.AssignedRoute)
	}
	if redeemed.MainServer != assignment.Server {
		t.Errorf("main server = %#v, want %#v", redeemed.MainServer, assignment.Server)
	}
	if redeemed.InvitePrivateKey != invited.InvitePrivateKey {
		t.Errorf("invite key = %q, want preserved %q", redeemed.InvitePrivateKey, invited.InvitePrivateKey)
	}
	if redeemed.MainPrivateKey != invited.MainPrivateKey {
		t.Errorf("main key = %q, want preserved %q", redeemed.MainPrivateKey, invited.MainPrivateKey)
	}
}

func TestRedeemInstall_ExactRetrySucceedsUnchanged(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	first := redeemInstall(t, db, "testnet")

	second, err := db.RedeemInstall("testnet", defaultNetworkAssignment())
	if err != nil {
		t.Fatalf("retry redeem: %v", err)
	}
	if second.MainAssignedRoute != first.MainAssignedRoute || second.MainServer != first.MainServer {
		t.Errorf("retry result = %#v, want unchanged %#v", second, first)
	}
}

func TestRedeemInstall_IncompatibleRetryLeavesRedemptionUnchanged(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	first := redeemInstall(t, db, "testnet")

	incompatible := defaultNetworkAssignment()
	incompatible.AssignedRoute = "10.42.0.6/32"
	_, err := db.RedeemInstall("testnet", incompatible)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	persisted, err := db.GetInstall("testnet")
	if err != nil {
		t.Fatalf("get redeemed install: %v", err)
	}
	if persisted.MainAssignedRoute != first.MainAssignedRoute {
		t.Errorf("main route = %q, want unchanged %q", persisted.MainAssignedRoute, first.MainAssignedRoute)
	}
}

func TestRedeemInstall_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.RedeemInstall("missing", defaultNetworkAssignment())
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRedeemInstall_CompletedNetworkReturnsInstallState(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	redeemInstall(t, db, "testnet")
	if _, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime); err != nil {
		t.Fatalf("confirm install: %v", err)
	}

	_, err := db.RedeemInstall("testnet", defaultNetworkAssignment())
	if !errors.Is(err, service.ErrInstallState) {
		t.Fatalf("err = %v, want ErrInstallState", err)
	}
}

func TestConfirmInstall_PromotesAuthoritativeInstallState(t *testing.T) {
	db := testutil.SetupDB(t)
	invited := beginInstall(t, db, "testnet")
	redeemed := redeemInstall(t, db, "testnet")
	confirmedAt := testutil.FixedTime.Add(time.Hour)

	network, err := db.ConfirmInstall("testnet", "main-priv-key", confirmedAt)
	if err != nil {
		t.Fatalf("confirm install: %v", err)
	}

	if network.Name != redeemed.Name {
		t.Errorf("name = %q, want %q", network.Name, redeemed.Name)
	}
	if network.PrivateKey != invited.MainPrivateKey {
		t.Errorf("private key = %q, want %q", network.PrivateKey, invited.MainPrivateKey)
	}
	if network.AssignedRoute != redeemed.MainAssignedRoute {
		t.Errorf("route = %q, want %q", network.AssignedRoute, redeemed.MainAssignedRoute)
	}
	if network.Server != redeemed.MainServer {
		t.Errorf("server = %#v, want %#v", network.Server, redeemed.MainServer)
	}
	if !network.Enabled {
		t.Error("confirmed network should be enabled")
	}
	if !network.CreatedAt.Equal(confirmedAt) {
		t.Errorf("created at = %v, want %v", network.CreatedAt, confirmedAt)
	}
	if _, err := db.GetInstall("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("install after confirm: err = %v, want ErrNotFound", err)
	}
}

func TestConfirmInstall_InvitedRejectedWithoutChange(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")

	_, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime)
	if !errors.Is(err, service.ErrInstallState) {
		t.Fatalf("err = %v, want ErrInstallState", err)
	}

	install, err := db.GetInstall("testnet")
	if err != nil {
		t.Fatalf("get install after rejection: %v", err)
	}
	if install.Phase != service.PhaseInvited {
		t.Errorf("phase = %q, want %q", install.Phase, service.PhaseInvited)
	}
	if _, err := db.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("network after rejection: err = %v, want ErrNotFound", err)
	}
}

func TestConfirmInstall_ExactRetryReturnsExistingNetwork(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	redeemInstall(t, db, "testnet")
	first, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime)
	if err != nil {
		t.Fatalf("first confirm: %v", err)
	}

	second, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime.Add(time.Hour))
	if err != nil {
		t.Fatalf("retry confirm: %v", err)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("retry created at = %v, want original %v", second.CreatedAt, first.CreatedAt)
	}
}

func TestConfirmInstall_IncompatibleRetryConflicts(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	redeemInstall(t, db, "testnet")
	first, err := db.ConfirmInstall(
		"testnet",
		"main-priv-key",
		testutil.FixedTime,
	)
	if err != nil {
		t.Fatalf("first confirm: %v", err)
	}

	_, err = db.ConfirmInstall(
		"testnet",
		"different-private-key",
		testutil.FixedTime.Add(time.Hour),
	)
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	persisted, err := db.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network after rejection: %v", err)
	}
	if persisted.PrivateKey != first.PrivateKey ||
		!persisted.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("network after rejection = %#v, want unchanged %#v", persisted, first)
	}
}

func TestConfirmInstall_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.ConfirmInstall("missing", "main-priv-key", testutil.FixedTime)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetworkState_Install(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")

	if err := db.DeleteNetworkState("testnet"); err != nil {
		t.Fatalf("delete install state: %v", err)
	}
	if _, err := db.GetInstall("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("install after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetworkState_Network(t *testing.T) {
	db := testutil.SetupDB(t)
	beginInstall(t, db, "testnet")
	redeemInstall(t, db, "testnet")
	if _, err := db.ConfirmInstall("testnet", "main-priv-key", testutil.FixedTime); err != nil {
		t.Fatalf("confirm install: %v", err)
	}

	if err := db.DeleteNetworkState("testnet"); err != nil {
		t.Fatalf("delete network state: %v", err)
	}
	if _, err := db.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("network after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetworkState_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.DeleteNetworkState("missing")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

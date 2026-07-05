package database_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func seedInstall(t *testing.T, db interface{ InsertInstall(*service.Install) error }, inst *service.Install) {
	t.Helper()
	if err := db.InsertInstall(inst); err != nil {
		t.Fatalf("seed install: %v", err)
	}
}

func defaultInstall(t *testing.T) *service.Install {
	t.Helper()
	return &service.Install{
		Name:                "testnet",
		Phase:               service.PhaseInvited,
		InviteIfaceName:     "testnet-i",
		InvitePrivateKey:    "invite-priv-key",
		InviteAssignedRoute: "10.42.1.5/32",
		InviteServer: service.ServerInfo{
			PublicKey: "invite-server-pubkey",
			Endpoint:  "1.2.3.4:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		MainIfaceName:  "testnet",
		MainPrivateKey: "main-priv-key",
		CreatedAt:      time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}
}

func TestGetInstall_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	_, err := db.GetInstall("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent install")
	}
}

func TestInsertAndGetInstall(t *testing.T) {
	db := testutil.SetupDB(t)

	createdAt := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	inst := &service.Install{
		Name:                "testnet",
		Phase:               service.PhaseInvited,
		InviteIfaceName:     "testnet-i",
		InvitePrivateKey:    "invite-priv-key",
		InviteAssignedRoute: "10.42.1.5/32",
		InviteServer: service.ServerInfo{
			PublicKey: "invite-server-pubkey",
			Endpoint:  "1.2.3.4:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		MainIfaceName:  "testnet",
		MainPrivateKey: "main-priv-key",
		CreatedAt:      createdAt,
	}

	if err := db.InsertInstall(inst); err != nil {
		t.Fatalf("insert install: %v", err)
	}

	got, err := db.GetInstall("testnet")
	if err != nil {
		t.Fatalf("get install: %v", err)
	}

	if got.Name != inst.Name {
		t.Errorf("name = %q, want %q", got.Name, inst.Name)
	}
	if got.Phase != inst.Phase {
		t.Errorf("phase = %q, want %q", got.Phase, inst.Phase)
	}
	if got.InviteIfaceName != inst.InviteIfaceName {
		t.Errorf("invite_iface_name = %q, want %q", got.InviteIfaceName, inst.InviteIfaceName)
	}
	if got.InvitePrivateKey != inst.InvitePrivateKey {
		t.Errorf("invite_private_key = %q, want %q", got.InvitePrivateKey, inst.InvitePrivateKey)
	}
	if got.InviteAssignedRoute != inst.InviteAssignedRoute {
		t.Errorf("invite_assigned_route = %q, want %q", got.InviteAssignedRoute, inst.InviteAssignedRoute)
	}
	if got.InviteServer.PublicKey != inst.InviteServer.PublicKey {
		t.Errorf("invite_server_pubkey = %q, want %q", got.InviteServer.PublicKey, inst.InviteServer.PublicKey)
	}
	if got.InviteServer.Endpoint != inst.InviteServer.Endpoint {
		t.Errorf("invite_server_endpoint = %q, want %q", got.InviteServer.Endpoint, inst.InviteServer.Endpoint)
	}
	if got.InviteServer.Route != inst.InviteServer.Route {
		t.Errorf("invite_server_route = %q, want %q", got.InviteServer.Route, inst.InviteServer.Route)
	}
	if got.InviteServer.APIPort != inst.InviteServer.APIPort {
		t.Errorf("invite_server_api_port = %d, want %d", got.InviteServer.APIPort, inst.InviteServer.APIPort)
	}
	if got.MainIfaceName != inst.MainIfaceName {
		t.Errorf("main_iface_name = %q, want %q", got.MainIfaceName, inst.MainIfaceName)
	}
	if got.MainPrivateKey != inst.MainPrivateKey {
		t.Errorf("main_private_key = %q, want %q", got.MainPrivateKey, inst.MainPrivateKey)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, createdAt)
	}
}

func TestInsertInstall_Duplicate(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	seedInstall(t, db, inst)

	err := db.InsertInstall(inst)
	if err == nil {
		t.Fatal("expected error for duplicate install")
	}
}

func TestListInstalls_Empty(t *testing.T) {
	db := testutil.SetupDB(t)

	installs, err := db.ListInstalls()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(installs) != 0 {
		t.Fatalf("expected 0 installs, got %d", len(installs))
	}
}

func TestListInstalls_Ordered(t *testing.T) {
	db := testutil.SetupDB(t)

	mustInsert := func(name string) {
		t.Helper()
		inst := defaultInstall(t)
		inst.Name = name
		seedInstall(t, db, inst)
	}

	mustInsert("beta")
	mustInsert("alpha")
	mustInsert("gamma")

	installs, err := db.ListInstalls()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(installs) != 3 {
		t.Fatalf("expected 3 installs, got %d", len(installs))
	}
	if installs[0].Name != "alpha" || installs[1].Name != "beta" || installs[2].Name != "gamma" {
		t.Fatalf("unexpected order: %v, %v, %v",
			installs[0].Name, installs[1].Name, installs[2].Name)
	}
}

func TestListInstalls_Multiple(t *testing.T) {
	db := testutil.SetupDB(t)

	alpha := defaultInstall(t)
	alpha.Name = "alpha"
	alpha.Phase = service.PhaseInvited
	seedInstall(t, db, alpha)

	beta := defaultInstall(t)
	beta.Name = "beta"
	beta.Phase = service.PhaseRedeemed
	seedInstall(t, db, beta)

	installs, err := db.ListInstalls()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(installs) != 2 {
		t.Fatalf("expected 2 installs, got %d", len(installs))
	}
	if installs[0].Name != "alpha" || installs[0].Phase != service.PhaseInvited {
		t.Errorf("alpha: name=%q phase=%q", installs[0].Name, installs[0].Phase)
	}
	if installs[1].Name != "beta" || installs[1].Phase != service.PhaseRedeemed {
		t.Errorf("beta: name=%q phase=%q", installs[1].Name, installs[1].Phase)
	}
}

func TestDeleteInstall(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	seedInstall(t, db, inst)

	if err := db.DeleteInstall(inst.Name); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := db.GetInstall(inst.Name)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteInstall_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.DeleteInstall("ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent install")
	}
}

func TestRedeemInstall_UpdatesPhaseAndMain(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	seedInstall(t, db, inst)

	mainServer := service.ServerInfo{
		PublicKey: "main-server-pubkey",
		Endpoint:  "5.6.7.8:51820",
		Route:     "10.42.0.1/32",
		APIPort:   8443,
	}

	if err := db.RedeemInstall(inst.Name, "10.42.1.10/32", mainServer); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	got, err := db.GetInstall(inst.Name)
	if err != nil {
		t.Fatalf("get after redeem: %v", err)
	}

	if got.Phase != service.PhaseRedeemed {
		t.Errorf("phase = %q, want %q", got.Phase, service.PhaseRedeemed)
	}
	if got.MainAssignedRoute != "10.42.1.10/32" {
		t.Errorf("main_assigned_route = %q, want %q", got.MainAssignedRoute, "10.42.1.10/32")
	}
	if got.MainServer.PublicKey != mainServer.PublicKey {
		t.Errorf("main_server_pubkey = %q, want %q", got.MainServer.PublicKey, mainServer.PublicKey)
	}
	if got.MainServer.Endpoint != mainServer.Endpoint {
		t.Errorf("main_server_endpoint = %q, want %q", got.MainServer.Endpoint, mainServer.Endpoint)
	}
	if got.MainServer.Route != mainServer.Route {
		t.Errorf("main_server_route = %q, want %q", got.MainServer.Route, mainServer.Route)
	}
	if got.MainServer.APIPort != mainServer.APIPort {
		t.Errorf("main_server_api_port = %d, want %d", got.MainServer.APIPort, mainServer.APIPort)
	}
}

func TestRedeemInstall_InviteFieldsPreserved(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	seedInstall(t, db, inst)

	mainServer := service.ServerInfo{
		PublicKey: "main-server-pubkey",
		Endpoint:  "5.6.7.8:51820",
		Route:     "10.42.0.1/32",
		APIPort:   8443,
	}

	if err := db.RedeemInstall(inst.Name, "10.42.1.10/32", mainServer); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	got, err := db.GetInstall(inst.Name)
	if err != nil {
		t.Fatalf("get after redeem: %v", err)
	}

	if got.InviteIfaceName != inst.InviteIfaceName {
		t.Errorf("invite_iface_name = %q, want %q", got.InviteIfaceName, inst.InviteIfaceName)
	}
	if got.InvitePrivateKey != inst.InvitePrivateKey {
		t.Errorf("invite_private_key = %q, want %q", got.InvitePrivateKey, inst.InvitePrivateKey)
	}
	if got.InviteAssignedRoute != inst.InviteAssignedRoute {
		t.Errorf("invite_assigned_route = %q, want %q", got.InviteAssignedRoute, inst.InviteAssignedRoute)
	}
	if got.InviteServer.PublicKey != inst.InviteServer.PublicKey {
		t.Errorf("invite_server_pubkey = %q, want %q", got.InviteServer.PublicKey, inst.InviteServer.PublicKey)
	}
	if got.InviteServer.Endpoint != inst.InviteServer.Endpoint {
		t.Errorf("invite_server_endpoint = %q, want %q", got.InviteServer.Endpoint, inst.InviteServer.Endpoint)
	}
	if got.InviteServer.Route != inst.InviteServer.Route {
		t.Errorf("invite_server_route = %q, want %q", got.InviteServer.Route, inst.InviteServer.Route)
	}
	if got.InviteServer.APIPort != inst.InviteServer.APIPort {
		t.Errorf("invite_server_api_port = %d, want %d", got.InviteServer.APIPort, inst.InviteServer.APIPort)
	}
	if got.MainPrivateKey != inst.MainPrivateKey {
		t.Errorf("main_private_key = %q, want %q", got.MainPrivateKey, inst.MainPrivateKey)
	}
}

func TestRedeemInstall_NotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	err := db.RedeemInstall("nonexistent", "10.42.1.10/32", service.ServerInfo{
		PublicKey: "pk",
		Endpoint:  "1.1.1.1:51820",
		Route:     "10.42.0.1/32",
		APIPort:   8443,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent install")
	}
}

func TestConfirmInstall_Success(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	inst.Phase = service.PhaseRedeemed
	inst.MainAssignedRoute = "10.42.1.10/32"
	inst.MainServer = service.ServerInfo{
		PublicKey: "main-server-pubkey",
		Endpoint:  "5.6.7.8:51820",
		Route:     "10.42.0.1/32",
		APIPort:   8443,
	}
	seedInstall(t, db, inst)

	createdAt := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	nc := &service.NetworkConfig{
		Name:          inst.Name,
		PrivateKey:    inst.MainPrivateKey,
		InterfaceName: inst.MainIfaceName,
		AssignedRoute: inst.MainAssignedRoute,
		Server:        inst.MainServer,
		Enabled:       true,
		CreatedAt:     createdAt,
	}

	if err := db.ConfirmInstall(inst.Name, nc); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	_, err := db.GetInstall(inst.Name)
	if err == nil {
		t.Fatal("expected install to be deleted after confirm")
	}

	gotNet, err := db.GetNetwork(inst.Name)
	if err != nil {
		t.Fatalf("get network after confirm: %v", err)
	}
	if gotNet.Name != nc.Name {
		t.Errorf("network name = %q, want %q", gotNet.Name, nc.Name)
	}
	if gotNet.PrivateKey != nc.PrivateKey {
		t.Errorf("network private_key = %q, want %q", gotNet.PrivateKey, nc.PrivateKey)
	}
	if gotNet.Enabled != nc.Enabled {
		t.Errorf("network enabled = %v, want %v", gotNet.Enabled, nc.Enabled)
	}
}

func TestConfirmInstall_InstallNotFound(t *testing.T) {
	db := testutil.SetupDB(t)

	nc := &service.NetworkConfig{
		Name:          "nonexistent",
		PrivateKey:    "priv",
		InterfaceName: "wg-nonexistent",
		AssignedRoute: "10.42.1.10/32",
		Server: service.ServerInfo{
			PublicKey: "pk",
			Endpoint:  "1.1.1.1:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		CreatedAt: time.Now(),
	}

	err := db.ConfirmInstall("nonexistent", nc)
	if err == nil {
		t.Fatal("expected error for nonexistent install")
	}
}

func TestConfirmInstall_RollbackOnNetworkConflict(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	inst.Phase = service.PhaseRedeemed
	inst.MainAssignedRoute = "10.42.1.10/32"
	inst.MainServer = service.ServerInfo{
		PublicKey: "main-server-pubkey",
		Endpoint:  "5.6.7.8:51820",
		Route:     "10.42.0.1/32",
		APIPort:   8443,
	}
	seedInstall(t, db, inst)

	// Pre-insert a network with the same name so the confirm insert conflicts.
	if err := db.InsertNetwork(&service.NetworkConfig{
		Name:          inst.Name,
		PrivateKey:    "existing-priv",
		InterfaceName: "wg-existing",
		AssignedRoute: "10.42.1.20/32",
		Server: service.ServerInfo{
			PublicKey: "existing-pk",
			Endpoint:  "9.9.9.9:51820",
			Route:     "10.42.0.1/32",
			APIPort:   8443,
		},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("insert existing network: %v", err)
	}

	nc := &service.NetworkConfig{
		Name:          inst.Name,
		PrivateKey:    inst.MainPrivateKey,
		InterfaceName: inst.MainIfaceName,
		AssignedRoute: inst.MainAssignedRoute,
		Server:        inst.MainServer,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}

	err := db.ConfirmInstall(inst.Name, nc)
	if err == nil {
		t.Fatal("expected error when network already exists")
	}

	// Install should survive the rollback.
	got, err := db.GetInstall(inst.Name)
	if err != nil {
		t.Fatalf("install should survive rolled-back confirm: %v", err)
	}
	if got.Phase != service.PhaseRedeemed {
		t.Errorf("phase = %q, want %q", got.Phase, service.PhaseRedeemed)
	}
}

func TestFullInstallLifecycle(t *testing.T) {
	db := testutil.SetupDB(t)

	inst := defaultInstall(t)
	seedInstall(t, db, inst)

	// --- Invited ---
	got, err := db.GetInstall(inst.Name)
	if err != nil {
		t.Fatalf("get invited: %v", err)
	}
	if got.Phase != service.PhaseInvited {
		t.Fatalf("expected invited phase, got %q", got.Phase)
	}

	// --- Redeem ---
	mainServer := service.ServerInfo{
		PublicKey: "main-server-pubkey",
		Endpoint:  "5.6.7.8:51820",
		Route:     "10.42.0.1/32",
		APIPort:   8443,
	}
	if err := db.RedeemInstall(inst.Name, "10.42.1.10/32", mainServer); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	got, err = db.GetInstall(inst.Name)
	if err != nil {
		t.Fatalf("get redeemed: %v", err)
	}
	if got.Phase != service.PhaseRedeemed {
		t.Fatalf("expected redeemed phase, got %q", got.Phase)
	}

	// --- Confirm ---
	nc := &service.NetworkConfig{
		Name:          inst.Name,
		PrivateKey:    inst.MainPrivateKey,
		InterfaceName: inst.MainIfaceName,
		AssignedRoute: got.MainAssignedRoute,
		Server:        got.MainServer,
		Enabled:       true,
		CreatedAt:     time.Now(),
	}
	if err := db.ConfirmInstall(inst.Name, nc); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	_, err = db.GetInstall(inst.Name)
	if err == nil {
		t.Fatal("install should be gone after confirm")
	}

	gotNet, err := db.GetNetwork(inst.Name)
	if err != nil {
		t.Fatalf("get network after confirm: %v", err)
	}
	if gotNet.Name != inst.Name {
		t.Errorf("network name = %q, want %q", gotNet.Name, inst.Name)
	}
}

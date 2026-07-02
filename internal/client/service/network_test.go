package service_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestGetNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "mynet")

	nw, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if nw.Name != "mynet" {
		t.Errorf("name = %q, want mynet", nw.Name)
	}
	if nw.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if nw.PublicKey == "" {
		t.Error("public_key should not be empty")
	}
	if nw.ServerPubkey == "" {
		t.Errorf("server_pubkey = %q, should not be empty", nw.ServerPubkey)
	}
	if nw.ServerEndpoint != "1.2.3.4:51820" {
		t.Errorf("server_endpoint = %q, want 1.2.3.4:51820", nw.ServerEndpoint)
	}
	if nw.ServerApiAddr != "10.42.0.1:8443" {
		t.Errorf("server_api_addr = %q, want 10.42.0.1:8443", nw.ServerApiAddr)
	}
	if nw.Enabled {
		t.Error("new network should be disabled")
	}
	if nw.CreatedAt.Unix() != testutil.FixedTime.Unix() {
		t.Errorf("created_at = %v, want %v", nw.CreatedAt, testutil.FixedTime)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks_Empty(t *testing.T) {
	env := testutil.SetupService(t)

	names, err := env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestListNetworks_WithNetworks(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "alpha")
	testutil.SeedNetworkDirect(t, env.Service, "beta")

	names, err := env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestInstall_Success(t *testing.T) {
	serverPubKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate server pub key: %v", err)
	}
	serverPubKeyStr := serverPubKey

	mux := http.NewServeMux()
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, serverapi.InvitationDTO{
			Network: serverapi.NetworkInfoDTO{
				PublicKey:   serverPubKeyStr,
				Endpoint:    "1.2.3.4:51820",
				APIEndpoint: r.Host,
			},
			Peer: serverapi.PeerIdentityDTO{
				CIDR: "10.42.0.5/16",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})

	env := testutil.SetupServiceWithServer(t, mux)
	inviteKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate invite key: %v", err)
	}
	invite := service.Invite{
		NetworkName:          "install-me",
		TempPeerPrivKey:      inviteKey,
		TempPeerAssignedCidr: "10.43.0.2/24",
		InviteServerPubkey:   serverPubKeyStr,
		InviteServerEndpoint: "5.6.7.8:51821",
		InviteServerAddr:     env.Server.Listener.Addr().String(),
	}

	nw, err := env.Service.Install(invite)
	if err != nil {
		t.Fatalf("install network: %v", err)
	}
	if nw.Name != "install-me" {
		t.Fatalf("name = %q, want install-me", nw.Name)
	}
	if nw.AssignedCidr != "10.42.0.5/16" {
		t.Fatalf("assigned cidr = %q, want 10.42.0.5/16", nw.AssignedCidr)
	}

	d := env.Backend.Device("install-me-i")
	if d == nil {
		t.Fatal("expected invite device was created")
	}
	if d.CloseCalls != 1 {
		t.Fatalf("invite down calls = %d, want 1", d.CloseCalls)
	}

	d2 := env.Backend.Device("install-me")
	if d2 == nil {
		t.Fatal("expected main device was created")
	}
	if d2.CloseCalls != 1 {
		t.Fatalf("main down calls = %d, want 1", d2.CloseCalls)
	}

	ops := env.Backend.AppliedOpsFor("install-me")
	var addOps []string
	for _, op := range ops {
		addOps = append(addOps, op.Config.PublicKey.String())
	}
	if len(addOps) != 1 {
		t.Fatalf("main peer ops = %d, want 1", len(addOps))
	}
	if nw.ServerEndpoint != "1.2.3.4:51820" {
		t.Fatalf("persisted ServerEndpoint = %q, want 1.2.3.4:51820", nw.ServerEndpoint)
	}
}

func TestInstall_Duplicate(t *testing.T) {
	t.Skip("requires mock HTTP server for /redeem endpoint")
}

func TestBeginInstall_MissingNetworkName(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		TempPeerPrivKey:      "temp-key",
		TempPeerAssignedCidr: "10.42.0.5/16",
		InviteServerPubkey:   "srv",
		InviteServerEndpoint: "1.2.3.4:51820",
		InviteServerAddr:     "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingTempPrivKey(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:          "noname",
		TempPeerAssignedCidr: "10.42.0.5/16",
		InviteServerPubkey:   "srv",
		InviteServerEndpoint: "1.2.3.4:51820",
		InviteServerAddr:     "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingTempCidr(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:          "noname",
		TempPeerPrivKey:      "temp-key",
		InviteServerPubkey:   "srv",
		InviteServerEndpoint: "1.2.3.4:51820",
		InviteServerAddr:     "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingServerPubkey(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:          "noname",
		TempPeerPrivKey:      "temp-key",
		TempPeerAssignedCidr: "10.42.0.5/16",
		InviteServerEndpoint: "1.2.3.4:51820",
		InviteServerAddr:     "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingServerEndpoint(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:          "noname",
		TempPeerPrivKey:      "temp-key",
		TempPeerAssignedCidr: "10.42.0.5/16",
		InviteServerPubkey:   "srv",
		InviteServerAddr:     "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingTempApiAddr(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:          "noname",
		TempPeerPrivKey:      "temp-key",
		TempPeerAssignedCidr: "10.42.0.5/16",
		InviteServerPubkey:   "srv",
		InviteServerEndpoint: "1.2.3.4:51820",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstall_PersistsKeys(t *testing.T) {
	t.Skip("requires mock HTTP server")
}

func TestUninstallNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "to-delete")

	if err := env.Service.UninstallNetwork("to-delete"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	_, err := env.Service.GetNetwork("to-delete")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after uninstall: err = %v, want ErrNotFound", err)
	}
}

func TestUninstallNetwork_DisablesFirst(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "enabled-net")

	ctx := context.Background()
	if err := env.Service.EnableNetwork(ctx, "enabled-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.Service.UninstallNetwork("enabled-net"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	d := env.Backend.Device("enabled-net")
	if d == nil {
		t.Fatal("expected device was created during enable")
	}
	if d.CloseCalls != 1 {
		t.Errorf("down calls = %d, want 1", d.CloseCalls)
	}
}

func TestEnableNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "enable-me")

	ctx := context.Background()
	if err := env.Service.EnableNetwork(ctx, "enable-me"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	nw, err := env.Service.GetNetwork("enable-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !nw.Enabled {
		t.Error("network should be enabled")
	}

	if env.Backend.Device("enable-me") == nil {
		t.Fatal("expected device was created")
	}
}

func TestEnableNetwork_AlreadyRunning(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "running")

	ctx := context.Background()
	if err := env.Service.EnableNetwork(ctx, "running"); err != nil {
		t.Fatalf("first enable: %v", err)
	}

	if err := env.Service.EnableNetwork(ctx, "running"); err != nil {
		t.Fatalf("second enable: %v", err)
	}

	if env.Backend.Device("running") == nil {
		t.Fatal("expected device was created")
	}
}

func TestEnableNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.EnableNetwork(context.Background(), "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEnableNetwork_DeviceError(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "bad-device")

	env.Backend.CreateErr = errors.New("device create failed")

	err := env.Service.EnableNetwork(context.Background(), "bad-device")
	if err == nil {
		t.Fatal("expected error")
	}

	nw, err := env.Service.GetNetwork("bad-device")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should stay disabled after failed enable")
	}
}

func TestDisableNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "disable-me")

	ctx := context.Background()
	if err := env.Service.EnableNetwork(ctx, "disable-me"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.Service.DisableNetwork("disable-me"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	nw, err := env.Service.GetNetwork("disable-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should be disabled")
	}

	d := env.Backend.Device("disable-me")
	if d == nil {
		t.Fatal("expected device was created")
	}
	if d.CloseCalls != 1 {
		t.Errorf("down calls = %d, want 1", d.CloseCalls)
	}
}

func TestDisableNetwork_NotEnabled(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "not-enabled")

	if err := env.Service.DisableNetwork("not-enabled"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	nw, err := env.Service.GetNetwork("not-enabled")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should still be disabled")
	}
}

func TestStatus_Empty(t *testing.T) {
	env := testutil.SetupService(t)

	statuses, err := env.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestStatus_WithInstalledNetworks(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "net-a")
	testutil.SeedNetworkDirect(t, env.Service, "net-b")

	statuses, err := env.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	for _, st := range statuses {
		if st.Enabled {
			t.Errorf("%s should not be enabled", st.Name)
		}
		if st.Running {
			t.Errorf("%s should not be running", st.Name)
		}
		if st.PeerCount != 0 {
			t.Errorf("%s peer_count = %d, want 0", st.Name, st.PeerCount)
		}
	}
}

func TestStatus_WithRunningNetworks(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "running-net")

	ctx := context.Background()
	if err := env.Service.EnableNetwork(ctx, "running-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	statuses, err := env.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	st := statuses[0]
	if st.Name != "running-net" {
		t.Errorf("name = %q, want running-net", st.Name)
	}
	if !st.Enabled {
		t.Error("expected enabled=true")
	}
	if !st.Running {
		t.Error("expected running=true")
	}
}

func TestFetchNetwork_NotRunning(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.FetchNetwork("any")
	if !errors.Is(err, service.ErrNetworkNotEnabled) {
		t.Errorf("err = %v, want ErrNetworkNotEnabled", err)
	}
}

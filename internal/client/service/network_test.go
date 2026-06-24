package service_test

import (
	"context"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func TestGetNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "mynet")

	nw, err := env.svc.GetNetwork("mynet")
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
	if nw.ServerPubkey != "server-pub-key" {
		t.Errorf("server_pubkey = %q, want server-pub-key", nw.ServerPubkey)
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
	if nw.CreatedAt.Unix() != fixedTime.Unix() {
		t.Errorf("created_at = %v, want %v", nw.CreatedAt, fixedTime)
	}
}

func TestGetNetwork_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks_Empty(t *testing.T) {
	env := setupTestEnv(t)

	names, err := env.svc.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestListNetworks_WithNetworks(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "alpha")
	seedNetwork(t, env.svc, "beta")

	names, err := env.svc.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestShowNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "shownet")

	nw, err := env.svc.ShowNetwork("shownet")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if nw.Name != "shownet" {
		t.Errorf("name = %q, want shownet", nw.Name)
	}
}

func TestShowNetwork_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.ShowNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestInstallNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)

	invite := service.Invite{
		NetworkName:    "testnet",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv-pub",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	}

	nw, err := env.svc.InstallNetwork(invite)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if nw.Name != "testnet" {
		t.Errorf("name = %q, want testnet", nw.Name)
	}
	if nw.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if nw.PublicKey == "" {
		t.Error("public_key should not be empty")
	}
	if nw.AssignedCidr != "10.42.0.5/16" {
		t.Errorf("assigned_cidr = %q, want 10.42.0.5/16", nw.AssignedCidr)
	}
	if nw.ServerPubkey != "srv-pub" {
		t.Errorf("server_pubkey = %q, want srv-pub", nw.ServerPubkey)
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
	if nw.CreatedAt.Unix() != fixedTime.Unix() {
		t.Errorf("created_at = %v, want %v", nw.CreatedAt, fixedTime)
	}
}

func TestInstallNetwork_Duplicate(t *testing.T) {
	env := setupTestEnv(t)

	invite := service.Invite{
		NetworkName:    "dup",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	}

	_, err := env.svc.InstallNetwork(invite)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}

	_, err = env.svc.InstallNetwork(invite)
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Errorf("err = %v, want ErrNetworkExists", err)
	}
}

func TestInstallNetwork_MissingNetworkName(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.InstallNetwork(service.Invite{
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstallNetwork_MissingServerPubkey(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.InstallNetwork(service.Invite{
		NetworkName:    "noname",
		AssignedCidr:   "10.42.0.5/16",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstallNetwork_MissingServerEndpoint(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.InstallNetwork(service.Invite{
		NetworkName:   "noname",
		AssignedCidr:  "10.42.0.5/16",
		ServerPubkey:  "srv",
		ServerApiAddr: "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstallNetwork_MissingServerApiAddr(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.InstallNetwork(service.Invite{
		NetworkName:    "noname",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv",
		ServerEndpoint: "1.2.3.4:51820",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstallNetwork_MissingAssignedCidr(t *testing.T) {
	env := setupTestEnv(t)

	_, err := env.svc.InstallNetwork(service.Invite{
		NetworkName:    "noname",
		ServerPubkey:   "srv",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	})
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstallNetwork_PersistsKeys(t *testing.T) {
	env := setupTestEnv(t)

	nw, err := env.svc.InstallNetwork(service.Invite{
		NetworkName:    "keys",
		AssignedCidr:   "10.42.0.5/16",
		ServerPubkey:   "srv-pub",
		ServerEndpoint: "1.2.3.4:51820",
		ServerApiAddr:  "10.42.0.1:8443",
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	got, err := env.svc.GetNetwork("keys")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PrivateKey != nw.PrivateKey {
		t.Errorf("private_key = %q, want %q", got.PrivateKey, nw.PrivateKey)
	}
	if got.PublicKey != nw.PublicKey {
		t.Errorf("public_key = %q, want %q", got.PublicKey, nw.PublicKey)
	}
}

func TestUninstallNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "to-delete")

	if err := env.svc.UninstallNetwork("to-delete"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	_, err := env.svc.GetNetwork("to-delete")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after uninstall: err = %v, want ErrNotFound", err)
	}
}

func TestUninstallNetwork_DisablesFirst(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "enabled-net")

	ctx := context.Background()
	if err := env.svc.EnableNetwork(ctx, "enabled-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.svc.UninstallNetwork("enabled-net"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	// Verify the device was cleaned up
	d, ok := env.wg.Devices["enabled-net"]
	if !ok {
		t.Fatal("expected device was created during enable")
	}
	if d.DownCalls != 1 {
		t.Errorf("down calls = %d, want 1", d.DownCalls)
	}
}

func TestEnableNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "enable-me")

	ctx := context.Background()
	if err := env.svc.EnableNetwork(ctx, "enable-me"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Verify network is now enabled in store
	nw, err := env.svc.GetNetwork("enable-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !nw.Enabled {
		t.Error("network should be enabled")
	}

	// Verify device was created
	d, ok := env.wg.Devices["enable-me"]
	if !ok {
		t.Fatal("expected device was created")
	}
	if d.UpCalls != 1 {
		t.Errorf("up calls = %d, want 1", d.UpCalls)
	}
}

func TestEnableNetwork_AlreadyRunning(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "running")

	ctx := context.Background()
	if err := env.svc.EnableNetwork(ctx, "running"); err != nil {
		t.Fatalf("first enable: %v", err)
	}

	// Second enable should be idempotent
	if err := env.svc.EnableNetwork(ctx, "running"); err != nil {
		t.Fatalf("second enable: %v", err)
	}

	d, ok := env.wg.Devices["running"]
	if !ok {
		t.Fatal("expected device was created")
	}
	if d.UpCalls != 1 {
		t.Errorf("up calls = %d, want 1 (idempotent)", d.UpCalls)
	}
}

func TestEnableNetwork_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	err := env.svc.EnableNetwork(context.Background(), "ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEnableNetwork_DeviceError(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "bad-device")

	env.wg.NewErr = errors.New("device create failed")

	err := env.svc.EnableNetwork(context.Background(), "bad-device")
	if err == nil {
		t.Fatal("expected error")
	}

	// Network should remain disabled in store
	nw, err := env.svc.GetNetwork("bad-device")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should stay disabled after failed enable")
	}
}

func TestDisableNetwork_Success(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "disable-me")

	ctx := context.Background()
	if err := env.svc.EnableNetwork(ctx, "disable-me"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.svc.DisableNetwork("disable-me"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	// Network should be disabled in store
	nw, err := env.svc.GetNetwork("disable-me")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should be disabled")
	}

	// Device should have been cleaned up
	d, ok := env.wg.Devices["disable-me"]
	if !ok {
		t.Fatal("expected device was created")
	}
	if d.DownCalls != 1 {
		t.Errorf("down calls = %d, want 1", d.DownCalls)
	}
}

func TestDisableNetwork_NotEnabled(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "not-enabled")

	if err := env.svc.DisableNetwork("not-enabled"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	nw, err := env.svc.GetNetwork("not-enabled")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if nw.Enabled {
		t.Error("network should still be disabled")
	}
}

func TestStatus_Empty(t *testing.T) {
	env := setupTestEnv(t)

	statuses, err := env.svc.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestStatus_WithInstalledNetworks(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "net-a")
	seedNetwork(t, env.svc, "net-b")

	statuses, err := env.svc.Status()
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
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "running-net")

	ctx := context.Background()
	if err := env.svc.EnableNetwork(ctx, "running-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	statuses, err := env.svc.Status()
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

func TestFetchNetwork_NotImplemented(t *testing.T) {
	env := setupTestEnv(t)

	err := env.svc.FetchNetwork("any")
	if !errors.Is(err, service.ErrNotImplemented) {
		t.Errorf("err = %v, want ErrNotImplemented", err)
	}
}

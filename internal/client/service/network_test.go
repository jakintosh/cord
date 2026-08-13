package service_test

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestGetNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "mynet")

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
	if pub, _ := wireguard.PublicKey(nw.PrivateKey); pub == "" {
		t.Error("public_key should not be empty")
	}
	if nw.Server.PublicKey == "" {
		t.Errorf("server_pubkey = %q, should not be empty", nw.Server.PublicKey)
	}
	if nw.Server.Endpoint != "1.2.3.4:51820" {
		t.Errorf("server_endpoint = %q, want 1.2.3.4:51820", nw.Server.Endpoint)
	}
	if nw.Server.Route != "10.42.0.1/32" {
		t.Errorf("server_route = %q, want 10.42.0.1/32", nw.Server.Route)
	}
	if nw.Server.APIPort != 8443 {
		t.Errorf("server_api_port = %d, want 8443", nw.Server.APIPort)
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
	testutil.SeedNetworkDirect(t, env.Database, "alpha")
	testutil.SeedNetworkDirect(t, env.Database, "beta")

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
		srvHost, srvPortStr, _ := net.SplitHostPort(r.Host)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   serverPubKeyStr,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				NetworkCidr: "10.0.0.0/16",
				APIPort:     uint16(srvPort),
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []protocol.VisiblePeer{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	inviteKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate invite key: %v", err)
	}
	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "install-me",
			PublicKey:   serverPubKeyStr,
			Endpoint:    "5.6.7.8:51821",
			ServerRoute: srvHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(srvPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: inviteKey,
		},
	}

	nc, err := env.Service.InstallNetwork(invite, installOptions())
	if err != nil {
		t.Fatalf("install network: %v", err)
	}
	if nc.Name != "install-me" {
		t.Fatalf("name = %q, want install-me", nc.Name)
	}
	if nc.AssignedRoute != "10.42.0.5/32" {
		t.Fatalf("assigned cidr = %q, want 10.42.0.5/32", nc.AssignedRoute)
	}

	d := env.Backend.Device("install-me-i")
	if d == nil {
		t.Fatal("expected invite device was created")
	}
	if d.CloseCalls != 1 {
		t.Fatalf("invite down calls = %d, want 1", d.CloseCalls)
	}
	inviteOps := d.AppliedOps()
	if len(inviteOps) != 1 {
		t.Fatalf("invite peer ops = %d, want 1", len(inviteOps))
	}
	if inviteOps[0].Target.PersistentKeepalive != 0 {
		t.Errorf("invite keepalive = %v, want disabled", inviteOps[0].Target.PersistentKeepalive)
	}

	d2 := env.Backend.Device("install-me")
	if d2 == nil {
		t.Fatal("expected main device was created")
	}
	if d2.CloseCalls != 0 {
		t.Fatalf("main down calls = %d, want 0 (network stays running)", d2.CloseCalls)
	}

	ops := env.Backend.AppliedOpsFor("install-me")
	var addOps []string
	for _, op := range ops {
		addOps = append(addOps, op.Target.PublicKey.String())
	}
	if len(addOps) != 1 {
		t.Fatalf("main peer ops = %d, want 1", len(addOps))
	}
	if ops[0].Target.PersistentKeepalive != service.PersistentKeepaliveInterval {
		t.Errorf("main server keepalive = %v, want %v", ops[0].Target.PersistentKeepalive, service.PersistentKeepaliveInterval)
	}
	if nc.Server.Endpoint != "1.2.3.4:51820" {
		t.Fatalf("persisted ServerEndpoint = %q, want 1.2.3.4:51820", nc.Server.Endpoint)
	}
}

func TestInstall_Duplicate(t *testing.T) {
	t.Skip("requires mock HTTP server for /redeem endpoint")
}

func TestBeginInstall_MissingNetworkName(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(protocol.Invitation{
		Network: protocol.NetworkInfo{
			PublicKey:   "srv",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.42.0.5/16",
			PrivateKey: "temp-key",
		},
	}, installOptions())
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingTempPrivKey(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "noname",
			PublicKey:   "srv",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route: "10.42.0.5/16",
		},
	}, installOptions())
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingTempCidr(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "noname",
			PublicKey:   "srv",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			PrivateKey: "temp-key",
		},
	}, installOptions())
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingServerPubkey(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "noname",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.42.0.5/16",
			PrivateKey: "temp-key",
		},
	}, installOptions())
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingServerEndpoint(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "noname",
			PublicKey:   "srv",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.42.0.5/16",
			PrivateKey: "temp-key",
		},
	}, installOptions())
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBeginInstall_MissingTempApiAddr(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "noname",
			PublicKey:   "srv",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.0.0.0/16",
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.42.0.5/16",
			PrivateKey: "temp-key",
		},
	}, installOptions())
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestInstall_PersistsKeys(t *testing.T) {
	t.Skip("requires mock HTTP server")
}

func TestUninstallNetwork_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "to-delete")

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
	testutil.SeedNetworkDirect(t, env.Database, "enabled-net")

	if err := env.Service.EnableNetwork("enabled-net"); err != nil {
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
	testutil.SeedNetworkDirect(t, env.Database, "enable-me")

	if err := env.Service.EnableNetwork("enable-me"); err != nil {
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
	testutil.SeedNetworkDirect(t, env.Database, "running")

	if err := env.Service.EnableNetwork("running"); err != nil {
		t.Fatalf("first enable: %v", err)
	}

	if err := env.Service.EnableNetwork("running"); err != nil {
		t.Fatalf("second enable: %v", err)
	}

	if env.Backend.Device("running") == nil {
		t.Fatal("expected device was created")
	}
}

func TestEnableNetwork_NotFound(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.EnableNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestEnableNetwork_DeviceError(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "bad-device")

	env.Backend.CreateErr = errors.New("device create failed")

	err := env.Service.EnableNetwork("bad-device")
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
	testutil.SeedNetworkDirect(t, env.Database, "disable-me")

	if err := env.Service.EnableNetwork("disable-me"); err != nil {
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
	testutil.SeedNetworkDirect(t, env.Database, "not-enabled")

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

func TestIsNetworkRunning_WithoutNetworks(t *testing.T) {
	env := testutil.SetupService(t)

	if env.Service.IsNetworkRunning("nonexistent") {
		t.Error("nonexistent network should not be running")
	}
}

func TestIsNetworkRunning_InstalledNotEnabled(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "net-a")

	if env.Service.IsNetworkRunning("net-a") {
		t.Error("non-enabled network should not be running")
	}
}

func TestIsNetworkRunning_EnabledNetwork(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "running-net")

	if err := env.Service.EnableNetwork("running-net"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if !env.Service.IsNetworkRunning("running-net") {
		t.Error("enabled network should be running")
	}
}

func TestFetchNetwork_NotRunning(t *testing.T) {
	env := testutil.SetupService(t)

	err := env.Service.SyncNetwork("any")
	if !errors.Is(err, service.ErrNetworkNotEnabled) {
		t.Errorf("err = %v, want ErrNetworkNotEnabled", err)
	}
}

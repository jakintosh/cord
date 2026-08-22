package api_test

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

//
// List
//

func TestAPIListNetworks_Empty(
	t *testing.T,
) {
	env := testutil.Setup(t)

	url := "/networks"
	result := wire.TestGet[[]string](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data))
	}
}

func TestAPIListNetworks_WithData(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t, "net-a")
	env.SeedNetwork(t, "net-b")

	url := "/networks"
	result := wire.TestGet[[]string](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(data))
	}
	names := make(map[string]bool, len(data))
	for _, n := range data {
		names[n] = true
	}
	if !names["net-a"] {
		t.Fatal("net-a not found in list")
	}
	if !names["net-b"] {
		t.Fatal("net-b not found in list")
	}
}

//
// Show
//

func TestAPIShowNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// show network
	url := "/networks/mynet"
	result := wire.TestGet[api.Network](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", data.Name)
	}
	if data.State != "installed" {
		t.Fatalf("state = %q, want installed", data.State)
	}
	if data.Enabled {
		t.Fatal("expected enabled=false for new network")
	}
}

func TestAPIShowNetwork_IncludesEnrichedFields(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	nc := env.SeedNetwork(t, "mynet")

	// show network
	url := "/networks/mynet"
	result := wire.TestGet[api.Network](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Address != nc.AssignedRoute {
		t.Fatalf("address = %q, want %q", data.Address, nc.AssignedRoute)
	}
	if data.Interface != nc.InterfaceName {
		t.Fatalf("interface = %q, want %q", data.Interface, nc.InterfaceName)
	}
	if data.ServerEndpoint != nc.Server.Endpoint {
		t.Fatalf("server_endpoint = %q, want %q", data.ServerEndpoint, nc.Server.Endpoint)
	}
}

func TestAPIShowNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// show nonexistent network
	url := "/networks/ghost"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIShowNetwork_MidInstall(
	t *testing.T,
) {
	// setup env with a server that answers /redeem
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)

	inst, err := env.Service.BeginInstall(installInvite(env, "mid-install"), service.NetworkOptions{})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Runtime.RedeemInstall(t.Context(), inst.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// show mid-install network
	url := "/networks/mid-install"
	result := wire.TestGet[api.Network](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "mid-install" {
		t.Fatalf("name = %q, want mid-install", data.Name)
	}
	if data.State != "redeemed" {
		t.Fatalf("state = %q, want redeemed", data.State)
	}
}

//
// Install
//

func TestAPIInstallNetwork_Success(
	t *testing.T,
) {
	tempKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate temp key: %v", err)
	}
	srvPub, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate server pub key: %v", err)
	}

	env := testutil.SetupWithServer(t, testutil.NewInstallServer)
	apiAddr := env.Server.Listener.Addr().String()
	apiHost, apiPortStr, _ := net.SplitHostPort(apiAddr)
	apiPort, _ := strconv.Atoi(apiPortStr)

	url := "/networks"
	body := `{
		"invitation": {
			"network": {
				"name": "mynet",
				"public_key": "` + srvPub + `",
				"endpoint": "1.2.3.4:51820",
				"server_route": "` + apiHost + `/32",
				"network_cidr": "10.42.0.0/16",
				"api_port": ` + strconv.Itoa(apiPort) + `
			},
			"peer": {
				"route": "10.42.0.5/16",
				"private_key": "` + tempKey + `"
			}
		},
		"listen_port": 51820
	}`
	result := wire.TestPost[api.Network](env.Router, url, body)

	data := result.ExpectStatusOK(t, http.StatusCreated)
	if data.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", data.Name)
	}
	if data.State != "installed" {
		t.Fatalf("state = %q, want installed", data.State)
	}
	if !data.Enabled {
		t.Fatal("expected enabled=true for new network")
	}

	nw, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if nw.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", nw.Name)
	}
	// The main network's identity comes from the redemption, not from
	// the invite the install started with.
	if nw.Server.PublicKey == "" {
		t.Fatal("server public_key should not be empty")
	}
	if nw.Server.Endpoint != "1.2.3.4:51820" {
		t.Fatalf("server endpoint = %q, want 1.2.3.4:51820", nw.Server.Endpoint)
	}
	if nw.Server.Route != apiHost+"/32" {
		t.Fatalf("server route = %q, want %q", nw.Server.Route, apiHost+"/32")
	}
	if nw.Server.APIPort != uint16(apiPort) {
		t.Fatalf("server api_port = %d, want %d", nw.Server.APIPort, apiPort)
	}
	if nw.ListenPort != 51820 {
		t.Fatalf("listen_port = %d, want 51820", nw.ListenPort)
	}
	if nw.PrivateKey == "" {
		t.Fatal("private_key should not be empty")
	}
}

func TestAPIUpdateNetwork(t *testing.T) {
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	result := wire.TestPatch[api.Network](env.Router, "/networks/mynet", `{"listen_port":51820}`)
	result.ExpectOK(t)
	if result.Data.ListenPort != 51820 {
		t.Errorf("response listen_port = %d, want 51820", result.Data.ListenPort)
	}

	network, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if network.ListenPort != 51820 {
		t.Errorf("listen_port = %d, want 51820", network.ListenPort)
	}
}

func TestAPIUpdateNetwork_Empty(t *testing.T) {
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	result := wire.TestPatch[api.Network](env.Router, "/networks/mynet", `{}`)
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIInstallNetwork_InvalidJSON(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post garbage
	url := "/networks"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIInstallNetwork_MissingName(
	t *testing.T,
) {
	env := testutil.Setup(t)

	url := "/networks"
	body := `{
		"invitation": {
			"network": {
				"public_key": "srv-pub",
				"endpoint": "1.2.3.4:51820",
				"server_route": "10.42.0.1/32",
				"api_port": 8443
			},
			"peer": {
				"route": "10.42.0.5/16",
				"private_key": "test-temp-key"
			}
		}
	}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIInstallNetwork_MalformedInvitation(
	t *testing.T,
) {
	env := testutil.Setup(t)

	// well-formed JSON but not a valid invitation object: the daemon
	// parses the payload itself and must reject it with a clean 400.
	url := "/networks"
	body := `["not", "an", "invitation"]`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIInstallNetwork_Duplicate(
	t *testing.T,
) {
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)
	env.SeedNetwork(t, "dupnet")

	url := "/networks"
	body := `{
		"invitation": {
			"network": {
				"name": "dupnet",
				"public_key": "srv-pub",
				"endpoint": "1.2.3.4:51820",
				"server_route": "10.42.0.1/32",
				"network_cidr": "10.42.0.0/16",
				"api_port": 8443
			},
			"peer": {
				"route": "10.42.0.5/16",
				"private_key": "test-temp-key"
			}
		}
	}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusConflict)
}

//
// Redeem
//

func TestAPIRedeemNetwork_IncludesAssignedAddress(
	t *testing.T,
) {
	// setup env with a server that answers /redeem
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)

	inst, err := env.Service.BeginInstall(installInvite(env, "mid-install"), service.NetworkOptions{})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	// redeem via the HTTP handler
	url := "/networks/" + inst.Name + "/redeem"
	result := wire.TestPost[api.Network](env.Router, url, "")

	// verify result — the server-assigned address is real information the
	// caller didn't already know, so redeem keeps a response body
	data := result.ExpectOK(t)
	if data.Name != "mid-install" {
		t.Fatalf("name = %q, want mid-install", data.Name)
	}
	if data.State != "redeemed" {
		t.Fatalf("state = %q, want redeemed", data.State)
	}
	if data.Address != "10.42.0.5/32" {
		t.Fatalf("address = %q, want 10.42.0.5/32", data.Address)
	}
}

//
// Confirm
//

func TestAPIConfirmNetwork_Success(
	t *testing.T,
) {
	// the install server answers /redeem and /confirm with a real
	// wireguard key, since confirm configures a live tunnel peer
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)

	inst, err := env.Service.BeginInstall(
		installInvite(env, "mid-install"),
		service.NetworkOptions{},
	)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Runtime.RedeemInstall(t.Context(), inst.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// confirm via the HTTP handler
	url := "/networks/" + inst.Name + "/confirm"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify network is installed in store
	nw, err := env.Service.GetNetwork("mid-install")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("expected network to be enabled after confirm")
	}
}

func TestAPIConfirmNetwork_InvitedReturnsConflict(
	t *testing.T,
) {
	// setup invited install
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)
	inst, err := env.Service.BeginInstall(
		installInvite(env, "still-invited"),
		service.NetworkOptions{},
	)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	// confirm before redemption
	url := "/networks/" + inst.Name + "/confirm"
	result := wire.TestPost[any](env.Router, url, "")

	// verify persisted-state conflict
	result.ExpectStatusError(t, http.StatusConflict)
}

//
// Uninstall
//

func TestAPIUninstallNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "to-delete")

	// uninstall network
	url := "/networks/to-delete"
	result := wire.TestDelete[any](env.Router, url)

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify network is gone
	_, err := env.Service.GetNetwork("to-delete")
	if err == nil {
		t.Fatal("expected not found after uninstall")
	}
}

func TestAPIUninstallNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// uninstall nonexistent network
	url := "/networks/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

//
// Enable
//

func TestAPIEnableNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "enable-me")

	// enable network
	url := "/networks/enable-me/enable"
	result := wire.TestPost[api.NetworkStatus](env.Router, url, "")

	// verify result — the recorded intent and what the daemon is doing
	data := result.ExpectOK(t)
	if !data.Enabled || !data.Running {
		t.Fatalf("status = %+v, want enabled and running", data)
	}
	if data.Reason != "" {
		t.Fatalf("reason = %q, want empty", data.Reason)
	}

	// verify network is enabled in store
	nw, err := env.Service.GetNetwork("enable-me")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("expected network to be enabled in store")
	}
}

// TestAPIEnableNetwork_ReportsDivergence verifies that a network that
// cannot be brought up stays enabled and explains itself, rather than
// failing the request and reverting the operator's intent.
func TestAPIEnableNetwork_ReportsDivergence(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t, "wont-start")
	env.Backend.CreateErr = errors.New("no such device")

	url := "/networks/wont-start/enable"
	result := wire.TestPost[api.NetworkStatus](env.Router, url, "")

	data := result.ExpectOK(t)
	if !data.Enabled || data.Running {
		t.Fatalf("status = %+v, want enabled and not running", data)
	}
	if data.Reason == "" {
		t.Fatal("status reason should explain the divergence")
	}

	nw, err := env.Service.GetNetwork("wont-start")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("a failed start must not un-set the enabled flag")
	}
}

func TestAPIEnableNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// enable nonexistent network
	url := "/networks/ghost/enable"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIEnableNetwork_AlreadyEnabled(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "already-on")

	// enable again — should be idempotent
	url := "/networks/already-on/enable"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify enabled in store
	nw, err := env.Service.GetNetwork("already-on")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("expected network to stay enabled in store")
	}
}

//
// Disable
//

func TestAPIDisableNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "disable-me")

	// disable network
	url := "/networks/disable-me/disable"
	result := wire.TestPost[api.NetworkStatus](env.Router, url, "")

	// verify result — the recorded intent and what the daemon is doing
	data := result.ExpectOK(t)
	if data.Enabled || data.Running {
		t.Fatalf("status = %+v, want disabled and stopped", data)
	}

	// verify network is disabled in store
	nw, err := env.Service.GetNetwork("disable-me")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if nw.Enabled {
		t.Fatal("expected network to be disabled in store")
	}
}

func TestAPIDisableNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// disable nonexistent network
	url := "/networks/ghost/disable"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIDisableNetwork_AlreadyDisabled(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "already-off")

	// disable again — should be idempotent
	url := "/networks/already-off/disable"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)
}

//
// helpers
//

// installInvite builds a valid invite whose invite-server route points at
// env's httptest server, so the invite tunnel's /redeem call actually
// reaches testutil.NewInstallServer.
func installInvite(
	env *testutil.APIEnv,
	networkName string,
) protocol.Invitation {
	return testutil.Invitation(networkName, env.Server.Listener.Addr().String())
}

//
// Fetch
//

func TestAPIFetchNetwork_NotEnabled(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// sync — returns conflict until enabled
	url := "/networks/mynet/sync"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result
	result.ExpectStatusError(t, http.StatusConflict)
}

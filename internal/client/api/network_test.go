package api_test

import (
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestAPIListNetworks_Empty(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// send request
	url := "/networks"
	result := wire.TestGet[[]string](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if len(data) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data))
	}
}

func TestAPIListNetworks_SortsNames(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "net-b")
	env.SeedNetwork(t, "net-a")

	// send request
	url := "/networks"
	result := wire.TestGet[[]string](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if len(data) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(data))
	}
	if !slices.Equal(data, []string{"net-a", "net-b"}) {
		t.Fatalf("networks = %v, want sorted names", data)
	}
}

func TestAPIShowNetwork_IncludesEnrichedFields(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	nc := env.SeedNetwork(t, "mynet")

	// send request
	url := "/networks/mynet"
	result := wire.TestGet[api.Network](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if data.Name != "mynet" || data.State != "installed" || data.Enabled {
		t.Fatalf("network = %+v, want installed disabled mynet", data)
	}
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

	// send request
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

	// send request
	url := "/networks/mid-install"
	result := wire.TestGet[api.Network](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if data.Name != "mid-install" {
		t.Fatalf("name = %q, want mid-install", data.Name)
	}
	if data.State != "redeemed" {
		t.Fatalf("state = %q, want redeemed", data.State)
	}
}

func TestAPIInstallNetwork_Success(
	t *testing.T,
) {
	// setup env
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

	// send request
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
	headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
	result := wire.TestPost[api.Network](env.Router, url, body, headers...)

	// verify response
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

func TestAPIUpdateNetwork_PersistsListenPort(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// send request
	url := "/networks/mynet"
	body := `{
  "listen_port": 51820
 }`
	headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
	result := wire.TestPatch[api.Network](env.Router, url, body, headers...)
	result.ExpectStatusOK(t, http.StatusOK)
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

func TestAPIUpdateNetwork_InvalidInputLeavesStateUnchanged(
	t *testing.T,
) {
	for _, tc := range []struct{ name, body string }{
		{"malformed", "{"},
		{"missing listen port", "{}"},
		{"out of range", `{"listen_port":65536}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// setup env
			env := testutil.Setup(t)
			before := env.SeedNetwork(t, "mynet")

			// send request
			url := "/networks/mynet"
			body := tc.body
			headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
			result := wire.TestPatch[any](env.Router, url, body, headers...)

			// verify response
			result.ExpectStatusError(t, http.StatusBadRequest)

			// verify rejected update
			after, err := env.Service.GetNetwork("mynet")
			if err != nil {
				t.Fatal(err)
			}
			if after.ListenPort != before.ListenPort {
				t.Fatalf("listen port changed: %d", after.ListenPort)
			}
		})
	}
}

func TestAPIInstallNetwork_InvalidInputCreatesNothing(
	t *testing.T,
) {
	for _, tc := range []struct{ name, body string }{
		{"malformed JSON", "{"},
		{"wrong shape", `["not", "an", "invitation"]`},
		{"missing invitation", "{}"},
		{"incomplete invitation", `{"invitation":{"network":{"name":"mynet"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// setup env
			env := testutil.Setup(t)

			// send request
			url := "/networks"
			body := tc.body
			headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
			result := wire.TestPost[any](env.Router, url, body, headers...)

			// verify response
			result.ExpectStatusError(t, http.StatusBadRequest)

			// verify no durable install or network
			networks, err := env.Service.ListNetworks()
			if err != nil {
				t.Fatal(err)
			}
			installs, err := env.Service.ListInstalls()
			if err != nil {
				t.Fatal(err)
			}
			if len(networks) != 0 || len(installs) != 0 {
				t.Fatalf("created networks %v or installs %v", networks, installs)
			}
		})
	}
}

func TestAPIInstallNetwork_Duplicate(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)
	env.SeedNetwork(t, "dupnet")

	// send request
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
	headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
	result := wire.TestPost[any](env.Router, url, body, headers...)

	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIRedeemNetwork_IncludesAssignedAddress(
	t *testing.T,
) {
	// setup env with a server that answers /redeem
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)

	inst, err := env.Service.BeginInstall(installInvite(env, "mid-install"), service.NetworkOptions{})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	// send request
	url := "/networks/" + inst.Name + "/redeem"
	body := ""
	result := wire.TestPost[api.Network](env.Router, url, body)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
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

	// send request
	url := "/networks/" + inst.Name + "/confirm"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify response
	result.ExpectStatusOK(t, http.StatusOK)

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

	// send request
	url := "/networks/" + inst.Name + "/confirm"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify persisted-state conflict
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIUninstallNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "to-delete")

	// send request
	url := "/networks/to-delete"
	result := wire.TestDelete[any](env.Router, url)

	// verify response
	result.ExpectStatusOK(t, http.StatusOK)

	// verify network is gone
	_, err := env.Service.GetNetwork("to-delete")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("get deleted network = %v, want not found", err)
	}
}

func TestAPIUninstallNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// send request
	url := "/networks/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIEnableNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "enable-me")

	// send request
	url := "/networks/enable-me/enable"
	body := ""
	result := wire.TestPost[api.NetworkStatus](env.Router, url, body)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if !data.Enabled || data.Health != "healthy" {
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
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "wont-start")
	env.Backend.CreateErr = errors.New("no such device")

	// send request
	url := "/networks/wont-start/enable"
	body := ""
	result := wire.TestPost[api.NetworkStatus](env.Router, url, body)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if !data.Enabled || data.Health != "degraded" {
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

	// send request
	url := "/networks/ghost/enable"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIEnableNetwork_AlreadyEnabled(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "already-on")

	// send request
	url := "/networks/already-on/enable"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify response
	result.ExpectStatusOK(t, http.StatusOK)

	// verify enabled in store
	nw, err := env.Service.GetNetwork("already-on")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("expected network to stay enabled in store")
	}
}

func TestAPIDisableNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedEnabledNetwork(t, "disable-me")

	// send request
	url := "/networks/disable-me/disable"
	body := ""
	result := wire.TestPost[api.NetworkStatus](env.Router, url, body)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if data.Enabled || data.Health != "inactive" {
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

	// send request
	url := "/networks/ghost/disable"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIDisableNetwork_AlreadyDisabled(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "already-off")

	// send request
	url := "/networks/already-off/disable"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify response
	result.ExpectStatusOK(t, http.StatusOK)
}

// installInvite builds a valid invite whose invite-server route points at
// env's httptest server, so the invite tunnel's /redeem call actually
// reaches testutil.NewInstallServer.
func installInvite(
	env *testutil.APIEnv,
	networkName string,
) protocol.Invitation {
	return testutil.Invitation(networkName, env.Server.Listener.Addr().String())
}

func TestAPISyncNetwork_DisabledReturnsConflict(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// send request
	url := "/networks/mynet/sync"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPINetworkMutation_MissingNetworkReturnsNotFound(
	t *testing.T,
) {
	for _, action := range []string{"redeem", "confirm"} {
		t.Run(action, func(t *testing.T) {
			// setup env
			env := testutil.Setup(t)

			// send request
			url := "/networks/missing/" + action
			body := ""
			result := wire.TestPost[any](env.Router, url, body)

			// verify response
			result.ExpectStatusError(t, http.StatusNotFound)
		})
	}
}

func TestAPISyncNetwork_RefreshesInstalledNetwork(
	t *testing.T,
) {
	// setup installed network with a reachable snapshot server
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)
	network, err := env.Runtime.Install(t.Context(), installInvite(env, "mynet"), service.NetworkOptions{})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	baseline := testutil.NetworkSnapshot()
	baseline.GeneratedAt = testutil.FixedTime.Add(-time.Hour)
	if err := env.Service.ApplyNetworkSnapshot("mynet", baseline); err != nil {
		t.Fatalf("seed older snapshot: %v", err)
	}

	// send request
	url := "/networks/mynet/sync"
	body := ""
	result := wire.TestPost[api.Network](env.Router, url, body)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if data.Name != network.Name || data.Address != network.AssignedRoute || !data.Enabled {
		t.Fatalf("synced network = %+v, want installed network %+v", data, network)
	}

	// verify the downloaded topology is available
	view, err := env.Runtime.GetNetworkTopology(t.Context(), "mynet")
	if err != nil {
		t.Fatalf("get synced topology: %v", err)
	}
	if !view.GeneratedAt.Equal(testutil.FixedTime) || !view.SyncedAt.Equal(testutil.FixedTime) {
		t.Fatalf("snapshot timestamps = %v, %v; want %v", view.GeneratedAt, view.SyncedAt, testutil.FixedTime)
	}
}

func TestAPISyncNetwork_MissingNetworkReturnsNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// send request
	url := "/networks/missing/sync"
	body := ""
	result := wire.TestPost[any](env.Router, url, body)

	// verify response
	result.ExpectStatusError(t, http.StatusNotFound)
}

package api_test

import (
	"net"
	"net/http"
	"strconv"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
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
	// setup env
	env := testutil.Setup(t)

	// list networks
	url := "/networks"
	result := wire.TestGet[[]api.NetworkDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data))
	}
}

func TestAPIListNetworks_WithData(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "net-a")
	env.SeedNetwork(t, "net-b")

	// list networks
	url := "/networks"
	result := wire.TestGet[[]api.NetworkDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(data))
	}
	names := make(map[string]bool, len(data))
	for _, n := range data {
		names[n.Name] = true
	}
	if !names["net-a"] {
		t.Fatal("net-a not found in list")
	}
	if !names["net-b"] {
		t.Fatal("net-b not found in list")
	}
}

func TestAPIListNetworks_IncludesRedeemedInstall(
	t *testing.T,
) {
	// setup env with a server that answers /redeem
	env := testutil.SetupWithServer(t, testutil.NewInstallServer)
	env.SeedNetwork(t, "net-a")

	inst, err := env.Service.BeginInstall(installInvite(t, env, "mid-install"))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Service.Redeem(inst.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// list networks
	url := "/networks"
	result := wire.TestGet[[]api.NetworkDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	states := make(map[string]string, len(data))
	for _, n := range data {
		states[n.Name] = n.State
	}
	if states["net-a"] != "installed" {
		t.Fatalf("net-a state = %q, want installed", states["net-a"])
	}
	if states["mid-install"] != "redeemed" {
		t.Fatalf("mid-install state = %q, want redeemed", states["mid-install"])
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
	result := wire.TestGet[api.NetworkDTO](env.Router, url)

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
	if data.Connected {
		t.Fatal("expected connected=false for disabled network")
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
	result := wire.TestGet[api.NetworkDTO](env.Router, url)

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
	if data.PeerCount != 0 {
		t.Fatalf("peer_count = %d, want 0", data.PeerCount)
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

	inst, err := env.Service.BeginInstall(installInvite(t, env, "mid-install"))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Service.Redeem(inst.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	// show mid-install network
	url := "/networks/mid-install"
	result := wire.TestGet[api.NetworkDTO](env.Router, url)

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

	handler := func(apiAddr string) http.Handler {
		return newInstallServer(apiAddr, srvPub)
	}
	env := testutil.SetupWithServer(t, handler)
	apiAddr := env.Server.Listener.Addr().String()
	apiHost, apiPortStr, _ := net.SplitHostPort(apiAddr)
	apiPort, _ := strconv.Atoi(apiPortStr)

	url := "/networks"
	body := `{
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
	}`
	result := wire.TestPost[api.NetworkDTO](env.Router, url, body)

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
	if !data.Connected {
		t.Fatal("expected connected=true: install adopts the live tunnel")
	}

	nw, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if nw.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", nw.Name)
	}
	if nw.Server.PublicKey != srvPub {
		t.Fatalf("server public_key = %q, want %q", nw.Server.PublicKey, srvPub)
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
	if nw.PrivateKey == "" {
		t.Fatal("private_key should not be empty")
	}
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

	inst, err := env.Service.BeginInstall(installInvite(t, env, "mid-install"))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	// redeem via the HTTP handler
	url := "/networks/" + inst.Name + "/redeem"
	result := wire.TestPost[api.NetworkDTO](env.Router, url, "")

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
	// setup env with a server that answers /redeem and /confirm using a
	// real wireguard key, since Confirm configures a live tunnel peer
	srvPub, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate server pub key: %v", err)
	}
	handler := func(apiAddr string) http.Handler {
		return newInstallServer(apiAddr, srvPub)
	}
	env := testutil.SetupWithServer(t, handler)

	tempKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate temp key: %v", err)
	}
	apiAddr := env.Server.Listener.Addr().String()
	apiHost, apiPortStr, _ := net.SplitHostPort(apiAddr)
	apiPort, _ := strconv.Atoi(apiPortStr)

	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "mid-install",
			PublicKey:   srvPub,
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: apiHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(apiPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.42.0.5/16",
			PrivateKey: tempKey,
		},
	}

	inst, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Service.Redeem(inst.Name); err != nil {
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
	result := wire.TestPost[any](env.Router, url, "")

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify network is enabled in store
	nw, err := env.Service.GetNetwork("enable-me")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("expected network to be enabled in store")
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
	result := wire.TestPost[any](env.Router, url, "")

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

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

func newInstallServer(
	apiAddr string,
	serverPubKey string,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		srvHost, srvPortStr, _ := net.SplitHostPort(apiAddr)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   serverPubKey,
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
	return mux
}

// installInvite builds a valid invite whose invite-server route points at
// env's httptest server, so the invite tunnel's /redeem call actually
// reaches testutil.NewInstallServer.
func installInvite(
	t *testing.T,
	env *testutil.APIEnv,
	networkName string,
) protocol.Invitation {
	t.Helper()

	tempKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate temp key: %v", err)
	}
	srvPub, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate server pub key: %v", err)
	}

	apiAddr := env.Server.Listener.Addr().String()
	apiHost, apiPortStr, _ := net.SplitHostPort(apiAddr)
	apiPort, _ := strconv.Atoi(apiPortStr)

	return protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        networkName,
			PublicKey:   srvPub,
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: apiHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(apiPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.42.0.5/16",
			PrivateKey: tempKey,
		},
	}
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

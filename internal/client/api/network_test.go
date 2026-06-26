package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
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
	if !data.Installed {
		t.Fatal("expected installed=true")
	}
	if data.Enabled {
		t.Fatal("expected enabled=false for new network")
	}
	if data.Connected {
		t.Fatal("expected connected=false for disabled network")
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

//
// Install
//

func TestAPIInstallNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// install network
	url := "/networks"
	body := `{
		"network_name": "mynet",
		"assigned_cidr": "10.42.0.5/16",
		"server_pubkey": "srv-pub",
		"server_endpoint": "1.2.3.4:51820",
		"server_api_addr": "10.42.0.1:8443"
	}`
	result := wire.TestPost[api.NetworkDTO](env.Router, url, body)

	// verify result
	data := result.ExpectStatusOK(t, http.StatusCreated)
	if data.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", data.Name)
	}
	if !data.Installed {
		t.Fatal("expected installed=true")
	}
	if data.Enabled {
		t.Fatal("expected enabled=false for new network")
	}
	if data.Connected {
		t.Fatal("expected connected=false for new network")
	}

	// verify network exists in store
	nw, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if nw.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", nw.Name)
	}
	if nw.ServerPubkey != "srv-pub" {
		t.Fatalf("server_pubkey = %q, want srv-pub", nw.ServerPubkey)
	}
	if nw.ServerEndpoint != "1.2.3.4:51820" {
		t.Fatalf("server_endpoint = %q, want 1.2.3.4:51820", nw.ServerEndpoint)
	}
	if nw.ServerApiAddr != "10.42.0.1:8443" {
		t.Fatalf("server_api_addr = %q, want 10.42.0.1:8443", nw.ServerApiAddr)
	}
	if nw.PrivateKey == "" {
		t.Fatal("private_key should not be empty")
	}
	if nw.PublicKey == "" {
		t.Fatal("public_key should not be empty")
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
	// setup env
	env := testutil.Setup(t)

	// post without network_name
	url := "/networks"
	body := `{
		"assigned_cidr": "10.42.0.5/16",
		"server_pubkey": "srv-pub",
		"server_endpoint": "1.2.3.4:51820",
		"server_api_addr": "10.42.0.1:8443"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIInstallNetwork_Duplicate(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "dupnet")

	// post duplicate
	url := "/networks"
	body := `{
		"network_name": "dupnet",
		"assigned_cidr": "10.42.0.5/16",
		"server_pubkey": "srv-pub",
		"server_endpoint": "1.2.3.4:51820",
		"server_api_addr": "10.42.0.1:8443"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
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
	result := wire.TestDelete[api.DeleteResponse](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", data.Status)
	}
	if data.ID != "to-delete" {
		t.Fatalf("id = %q, want to-delete", data.ID)
	}

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
	result := wire.TestPost[api.NetworkDTO](env.Router, url, "")

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "enable-me" {
		t.Fatalf("name = %q, want enable-me", data.Name)
	}
	if !data.Enabled {
		t.Fatal("expected enabled=true")
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
	result := wire.TestPost[api.NetworkDTO](env.Router, url, "")

	// verify result
	data := result.ExpectOK(t)
	if !data.Enabled {
		t.Fatal("expected enabled=true")
	}

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
	result := wire.TestPost[api.NetworkDTO](env.Router, url, "")

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "disable-me" {
		t.Fatalf("name = %q, want disable-me", data.Name)
	}
	if data.Enabled {
		t.Fatal("expected enabled=false")
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
	result := wire.TestPost[api.NetworkDTO](env.Router, url, "")

	// verify result
	data := result.ExpectOK(t)
	if data.Enabled {
		t.Fatal("expected enabled=false")
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

	// fetch — currently returns 501 until server tunnel is wired
	url := "/networks/mynet/fetch"
	result := wire.TestPost[any](env.Router, url, "")

	// verify result
	result.ExpectStatusError(t, http.StatusConflict)
}

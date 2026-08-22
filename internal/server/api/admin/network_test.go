package admin_test

import (
	"errors"
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPICreateNetwork_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post network
	url := "/networks"
	body := `{
		"name": "mynet",
		"external_ip": "1.2.3.4",
		"main_cidr": "10.0.0.0/16",
		"main_wg_port": 51820
	}`
	result := wire.TestPost[admin.Network](env.Router, url, body)

	// verify result
	data := result.ExpectStatusOK(t, http.StatusCreated)
	if data.Name != "mynet" {
		t.Fatalf("name = %q, want mynet", data.Name)
	}
	if data.MainCidr != "10.0.0.0/16" {
		t.Fatalf("cidr = %q, want 10.0.0.0/16", data.MainCidr)
	}
	if data.ExternalIP != "1.2.3.4" {
		t.Fatalf("external_ip = %q, want 1.2.3.4", data.ExternalIP)
	}
	if data.MainWgPort != 51820 {
		t.Fatalf("port = %d, want 51820", data.MainWgPort)
	}
	if data.InviteWgPort != 51821 {
		t.Fatalf("invite_wg_port = %d, want 51821", data.InviteWgPort)
	}
	if data.InviteCidr != "172.16.10.0/24" {
		t.Fatalf("invite_cidr = %q, want 172.16.10.0/24", data.InviteCidr)
	}
	if data.MainApiPort != 8080 {
		t.Fatalf("api_port = %d, want 8080", data.MainApiPort)
	}
	if data.InviteApiPort != 8080 {
		t.Fatalf("invite_api_port = %d, want 8080", data.InviteApiPort)
	}

	// verify network exists in store
	net, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatalf("get network from service: %v", err)
	}
	if net.Main.Cidr != "10.0.0.0/16" {
		t.Fatalf("main_cidr = %q, want 10.0.0.0/16", net.Main.Cidr)
	}
}

func TestAPICreateNetwork_WithAllFields(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post network with explicit invite fields
	url := "/networks"
	body := `{
		"name": "fullnet",
		"main_cidr": "10.0.0.0/16",
		"invite_cidr": "10.1.0.0/24",
		"external_ip": "1.2.3.4",
		"main_wg_port": 51820,
		"invite_wg_port": 51821,
		"main_api_port": 8080,
		"invite_api_port": 8080
	}`
	result := wire.TestPost[admin.Network](env.Router, url, body)

	// verify result
	data := result.ExpectStatusOK(t, http.StatusCreated)
	if data.InviteCidr != "10.1.0.0/24" {
		t.Fatalf("invite_cidr = %q, want 10.1.0.0/24", data.InviteCidr)
	}
	if data.MainApiPort != 8080 {
		t.Fatalf("api_port = %d, want 8080", data.MainApiPort)
	}
	if data.InviteApiPort != 8080 {
		t.Fatalf("invite_api_port = %d, want 8080", data.InviteApiPort)
	}
	if data.InviteWgPort != 51821 {
		t.Fatalf("invite_wg_port = %d, want 51821", data.InviteWgPort)
	}
}

func TestAPICreateNetwork_InvalidJSON(
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

func TestAPICreateNetwork_EmptyName(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post without name
	url := "/networks"
	body := `{
		"cidr": "10.0.0.0/16",
		"external_ip": "1.2.3.4",
		"port": 51820
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateNetwork_InvalidCIDR(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post with invalid cidr
	url := "/networks"
	body := `{
		"name": "badcidr",
		"cidr": "not-a-cidr",
		"external_ip": "1.2.3.4",
		"port": 51820
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateNetwork_CIDRWithHostBits(
	t *testing.T,
) {
	env := testutil.Setup(t)

	result := wire.TestPost[any](env.Router, "/networks", `{
		"name": "hostbits",
		"external_ip": "1.2.3.4",
		"main_cidr": "10.0.99.0/16"
	}`)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateNetwork_MissingExternalIP(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post without external_ip
	url := "/networks"
	body := `{
		"name": "noip",
		"cidr": "10.0.0.0/16",
		"port": 51820
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateNetwork_MissingPort(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post without port
	url := "/networks"
	body := `{
		"name": "noport",
		"cidr": "10.0.0.0/16",
		"external_ip": "1.2.3.4"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateNetwork_DuplicateName(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// post duplicate
	url := "/networks"
	body := `{
		"name": "testnet",
		"external_ip": "1.2.3.4",
		"main_cidr": "10.42.0.0/16",
		"main_wg_port": 51820
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIListNetworks_Empty(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// list networks
	url := "/networks"
	result := wire.TestGet[[]string](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data))
	}
}

func TestAPIListNetworks_WithData(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list networks
	url := "/networks"
	result := wire.TestGet[[]string](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data))
	}
	if data[0] != "testnet" {
		t.Fatalf("name = %q, want testnet", data[0])
	}
}

func TestAPIShowNetwork_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// get network
	url := "/networks/testnet"
	result := wire.TestGet[admin.Network](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "testnet" {
		t.Fatalf("name = %q, want testnet", data.Name)
	}
	if data.MainCidr != "10.0.0.0/16" {
		t.Fatalf("cidr = %q, want 10.0.0.0/16", data.MainCidr)
	}
	if data.ExternalIP != "192.168.1.1" {
		t.Fatalf("external_ip = %q, want 192.168.1.1", data.ExternalIP)
	}
	if data.MainWgPort != 51820 {
		t.Fatalf("port = %d, want 51820", data.MainWgPort)
	}
	if data.InviteWgPort != 51821 {
		t.Fatalf("invite_wg_port = %d, want 51821", data.InviteWgPort)
	}
}

func TestAPIShowNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get nonexistent network
	url := "/networks/ghost"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIDeleteNetwork_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// delete network
	url := "/networks/testnet"
	result := wire.TestDelete[any](env.Router, url)

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify network is gone
	_, err := env.Service.GetNetwork("testnet")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestAPIDeleteNetwork_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// delete nonexistent network
	url := "/networks/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIDeleteNetwork_RefusesEnabled(
	t *testing.T,
) {
	// setup env, seed and enable a network
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	wire.TestPost[admin.NetworkStatus](env.Router, "/networks/testnet/enable", "").ExpectOK(t)

	// delete the enabled network
	result := wire.TestDelete[any](env.Router, "/networks/testnet")

	// verify result
	result.ExpectStatusError(t, http.StatusConflict)
	if _, err := env.Service.GetNetwork("testnet"); err != nil {
		t.Fatalf("network should survive a refused delete: %v", err)
	}
}

func TestAPIEnableNetwork_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// enable network
	url := "/networks/testnet/enable"
	result := wire.TestPost[admin.NetworkStatus](env.Router, url, "")

	// verify result — the network is enabled and running, with no divergence
	data := result.ExpectOK(t)
	if !data.Enabled || !data.Running {
		t.Fatalf("status = %+v, want enabled and running", data)
	}
	if data.Reason != "" {
		t.Fatalf("reason = %q, want empty", data.Reason)
	}

	// verify network is enabled in store
	nw, err := env.Service.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !nw.Enabled {
		t.Fatal("expected network to be enabled in store")
	}
}

func TestAPIEnableNetwork_StartFailureKeepsIntent(
	t *testing.T,
) {
	// setup env and seed network, with a WireGuard backend that fails
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.Backend.CreateErr = errors.New("no such device")

	// enable network
	result := wire.TestPost[admin.NetworkStatus](env.Router, "/networks/testnet/enable", "")

	// verify result — the intent stands and the response explains reality
	data := result.ExpectOK(t)
	if !data.Enabled || data.Running {
		t.Fatalf("status = %+v, want enabled but not running", data)
	}
	if data.Reason == "" {
		t.Fatal("expected a reason for the divergence")
	}

	// verify the enabled flag survived the failed start
	nw, err := env.Service.GetNetwork("testnet")
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

func TestAPIDisableNetwork_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// enable then disable network
	enableURL := "/networks/testnet/enable"
	wire.TestPost[admin.NetworkStatus](env.Router, enableURL, "").ExpectOK(t)

	url := "/networks/testnet/disable"
	result := wire.TestPost[admin.NetworkStatus](env.Router, url, "")

	// verify result — the network is neither enabled nor running
	data := result.ExpectOK(t)
	if data.Enabled || data.Running {
		t.Fatalf("status = %+v, want disabled and stopped", data)
	}

	// verify network is disabled in store
	nw, err := env.Service.GetNetwork("testnet")
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

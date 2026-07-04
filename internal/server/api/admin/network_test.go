package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
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
	result := wire.TestPost[admin.NetworkDTO](env.Router, url, body)

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
	if data.MainApiPort != 80 {
		t.Fatalf("api_port = %d, want 80", data.MainApiPort)
	}
	if data.InviteApiPort != 80 {
		t.Fatalf("invite_api_port = %d, want 80", data.InviteApiPort)
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
	result := wire.TestPost[admin.NetworkDTO](env.Router, url, body)

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
	result := wire.TestGet[admin.NetworkDTO](env.Router, url)

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
	result := wire.TestDelete[api.DeleteResponse](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", data.Status)
	}
	if data.ID != "testnet" {
		t.Fatalf("id = %q, want testnet", data.ID)
	}

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

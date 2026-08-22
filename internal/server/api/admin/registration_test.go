package admin_test

import (
	"net"
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPICreateRegistration_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add peer
	url := "/networks/testnet/registrations"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5",
		"admin": false
	}`
	result := wire.TestPost[protocol.Invitation](env.Router, url, body)

	// verify result — handler returns an Invitation payload
	result.ExpectStatusOK(t, http.StatusCreated)
	if result.Data.Network.Name != "testnet" {
		t.Fatalf("network_name = %q, want testnet", result.Data.Network.Name)
	}
	if result.Data.Peer.PrivateKey == "" {
		t.Fatal("private_key should not be empty")
	}
	if result.Data.Peer.Route == "" {
		t.Fatal("route should not be empty")
	}
	if result.Data.Network.PublicKey == "" {
		t.Fatal("server public_key should not be empty")
	}
	if result.Data.Network.Endpoint == "" {
		t.Fatal("endpoint should not be empty")
	}
	if result.Data.Network.ServerRoute == "" {
		t.Fatal("server_route should not be empty")
	}
	if result.Data.Network.APIPort == 0 {
		t.Fatal("api_port should not be zero")
	}

	// verify registration was created
	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].Name != "alice" {
		t.Fatalf("registration name = %q, want alice", regs[0].Name)
	}
}

func TestAPICreateRegistration_AddressExhausted(t *testing.T) {
	env := testutil.Setup(t)
	if _, err := env.Service.CreateNetwork(
		"small",
		"192.168.1.1",
		service.Plane{Cidr: "10.0.0.0/24", WireguardPort: 51820, ApiPort: 8080},
		service.Plane{Cidr: "10.1.0.0/30", WireguardPort: 51821, ApiPort: 8080},
	); err != nil {
		t.Fatalf("create network: %v", err)
	}

	url := "/networks/small/registrations"
	wire.TestPost[protocol.Invitation](
		env.Router,
		url,
		`{"name":"first","ip":"10.0.0.5"}`,
	).ExpectStatusOK(t, http.StatusCreated)
	wire.TestPost[any](
		env.Router,
		url,
		`{"name":"second","ip":"10.0.0.6"}`,
	).ExpectStatusError(t, http.StatusConflict)
}

func TestAPICreateRegistration_MissingIP(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/registrations"
	body := `{
		"name": "bob",
		"admin": false
	}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateRegistration_InvalidJSON(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// post garbage
	url := "/networks/testnet/registrations"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateRegistration_NetworkNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// add peer to nonexistent network
	url := "/networks/ghost/registrations"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPICreateRegistration_DuplicateName(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add first peer
	url := "/networks/testnet/registrations"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	result := wire.TestPost[protocol.Invitation](env.Router, url, body)
	result.ExpectStatusOK(t, http.StatusCreated)

	// add duplicate
	result2 := wire.TestPost[any](env.Router, url, body)

	// verify result — should get conflict
	result2.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIListRegistrations_Empty(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list registrations
	url := "/networks/testnet/registrations"
	result := wire.TestGet[[]admin.Registration](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 registrations, got %d", len(data))
	}
}

func TestAPIListRegistrations_WithData(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add a peer which creates a registration
	url := "/networks/testnet/registrations"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	result := wire.TestPost[any](env.Router, url, body)
	result.ExpectStatusOK(t, http.StatusCreated)

	// list registrations
	url = "/networks/testnet/registrations"
	listResult := wire.TestGet[[]admin.Registration](env.Router, url)

	// verify result
	data := listResult.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(data))
	}
	if data[0].Name != "alice" {
		t.Fatalf("name = %q, want alice", data[0].Name)
	}
	if data[0].Redeemed {
		t.Fatal("registration should not be redeemed")
	}
	if data[0].ExpiresAt == "" {
		t.Fatal("expires_at should not be empty")
	}
	if data[0].Route == "" {
		t.Fatal("route should not be empty")
	}
	if data[0].Admin {
		t.Fatal("registration should not be admin")
	}
}

func TestAPIListRegistrations_NetworkNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// list registrations for nonexistent network — returns empty list
	url := "/networks/ghost/registrations"
	result := wire.TestGet[[]admin.Registration](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 registrations for nonexistent network, got %d", len(data))
	}
}

func TestAPIRevokeRegistration_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// create a registration
	createURL := "/networks/testnet/registrations"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	createResult := wire.TestPost[any](env.Router, createURL, body)
	createResult.ExpectStatusOK(t, http.StatusCreated)

	// revoke the registration
	url := "/networks/testnet/registrations/alice"
	result := wire.TestDelete[any](env.Router, url)

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify registration is gone
	listURL := "/networks/testnet/registrations"
	listResult := wire.TestGet[[]admin.Registration](env.Router, listURL)
	remaining := listResult.ExpectOK(t)
	if len(remaining) != 0 {
		t.Fatalf("expected 0 registrations after revoke, got %d", len(remaining))
	}
}

func TestAPIRevokeRegistration_NotFound(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// revoke nonexistent registration
	url := "/networks/testnet/registrations/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIRegistrationGroups_AddListRemove(t *testing.T) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedRegistrationGroupResources(t, env)

	url := "/networks/testnet/registrations/alice/groups"
	body := `{"group":"engineering"}`
	wire.TestPost[any](env.Router, url, body).ExpectStatusOK(t, http.StatusCreated)

	groups := wire.TestGet[[]admin.Group](env.Router, url).ExpectOK(t)
	if len(groups) != 1 || groups[0].Name != "engineering" {
		t.Fatalf("groups = %+v, want engineering", groups)
	}

	wire.TestPost[any](env.Router, url, body).ExpectStatusError(t, http.StatusConflict)
	wire.TestDelete[any](env.Router, url+"/engineering").ExpectOK(t)
	groups = wire.TestGet[[]admin.Group](env.Router, url).ExpectOK(t)
	if len(groups) != 0 {
		t.Fatalf("groups after removal = %+v, want empty", groups)
	}
}

func TestAPIRegistrationGroups_RejectsMissingAndConfirmed(t *testing.T) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedRegistrationGroupResources(t, env)

	missingURL := "/networks/testnet/registrations/missing/groups"
	wire.TestGet[[]admin.Group](env.Router, missingURL).ExpectStatusError(t, http.StatusNotFound)

	reg, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", reg[0].InvitePublicKey, "alice-key"); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	url := "/networks/testnet/registrations/alice/groups"
	body := `{"group":"engineering"}`
	wire.TestPost[any](env.Router, url, body).ExpectStatusError(t, http.StatusConflict)
	wire.TestDelete[any](env.Router, "/networks/testnet/registrations/alice").ExpectStatusError(t, http.StatusConflict)
}

func seedRegistrationGroupResources(t *testing.T, env *testutil.APIEnv) {
	t.Helper()
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := env.Service.CreateRegistration(
		"testnet",
		"alice",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
}

package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIInviteCreate_Success(
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
	result := wire.TestPost[service.Invitation](env.Router, url, body)

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

func TestAPIInviteCreate_AutoAssignIP(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add peer without explicit IP — should auto-assign
	url := "/networks/testnet/registrations"
	body := `{
		"name": "bob",
		"admin": false
	}`
	result := wire.TestPost[service.Invitation](env.Router, url, body)

	// verify result
	result.ExpectStatusOK(t, http.StatusCreated)
	if result.Data.Peer.Route == "" {
		t.Fatal("route should not be empty for auto-assigned IP")
	}
}

func TestAPIInviteCreate_InvalidJSON(
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

func TestAPIInviteCreate_NetworkNotFound(
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

func TestAPIInviteCreate_DuplicateName(
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
	result := wire.TestPost[service.Invitation](env.Router, url, body)
	result.ExpectStatusOK(t, http.StatusCreated)

	// add duplicate
	result2 := wire.TestPost[any](env.Router, url, body)

	// verify result — should get conflict
	result2.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIListPeers_Empty(
	t *testing.T,
) {
	// setup env and seed network (creates cord-server peer)
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list peers — should include the server peer
	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.PeerDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 peer (cord-server), got %d", len(data))
	}
	if data[0].Name != "cord-server" {
		t.Fatalf("name = %q, want cord-server", data[0].Name)
	}
}

func TestAPIListPeers_WithData(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// seed a peer through the database
	if err := env.Database.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Route:     "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// list peers
	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.PeerDTO](env.Router, url)

	// verify result — cord-server + alice
	data := result.ExpectOK(t)
	if len(data) != 2 {
		t.Fatalf("expected 2 peers (cord-server + alice), got %d", len(data))
	}
	foundAlice := false
	for _, p := range data {
		if p.Name == "alice" {
			foundAlice = true
			if p.PublicKey != "alice-pub-key" {
				t.Fatalf("public_key = %q, want alice-pub-key", p.PublicKey)
			}
			if p.Route == "" {
				t.Fatal("route should not be empty")
			}
			break
		}
	}
	if !foundAlice {
		t.Fatal("alice not found in peer list")
	}
}

func TestAPIRenamePeer_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// seed a peer
	if err := env.Database.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Route:     "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// rename peer
	url := "/networks/testnet/peers/alice"
	body := `{"name": "alicia"}`
	result := wire.TestPatch[admin.PeerDTO](env.Router, url, body)

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "alicia" {
		t.Fatalf("name = %q, want alicia", data.Name)
	}

	// verify peer was renamed in store
	_, err := env.Service.GetPeer("testnet", "alicia")
	if err != nil {
		t.Fatalf("get renamed peer: %v", err)
	}
}

func TestAPIRenamePeer_NotFound(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// rename nonexistent peer
	url := "/networks/testnet/peers/ghost"
	body := `{"name": "casper"}`
	result := wire.TestPatch[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIDeletePeer_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// seed a peer
	if err := env.Database.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Route:     "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// delete peer
	url := "/networks/testnet/peers/alice"
	result := wire.TestDelete[api.DeleteResponse](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", data.Status)
	}
	if data.ID != "alice" {
		t.Fatalf("id = %q, want alice", data.ID)
	}

	// verify peer is gone
	_, err := env.Service.GetPeer("testnet", "alice")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestAPIDeletePeer_NotFound(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// delete nonexistent peer
	url := "/networks/testnet/peers/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIEnablePeer_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// seed a disabled peer
	if err := env.Database.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Route:     "10.0.0.5/32",
		Admin:     false,
		Enabled:   false,
		Confirmed: false,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// enable peer
	url := "/networks/testnet/peers/alice/enable"
	result := wire.TestPost[admin.PeerDTO](env.Router, url, "")

	// verify result
	data := result.ExpectOK(t)
	if data.Name != "alice" {
		t.Fatalf("name = %q, want alice", data.Name)
	}
	if !data.Enabled {
		t.Fatal("expected peer to be enabled")
	}

	// verify peer is enabled in store
	peer, err := env.Service.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if !peer.Enabled {
		t.Fatal("expected peer to be enabled in store")
	}
}

func TestAPIDisablePeer_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// seed an enabled peer
	if err := env.Database.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Route:     "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// disable peer
	url := "/networks/testnet/peers/alice/disable"
	result := wire.TestPost[admin.PeerDTO](env.Router, url, "")

	// verify result
	data := result.ExpectOK(t)
	if data.Enabled {
		t.Fatal("expected peer to be disabled")
	}

	// verify peer is disabled in store
	peer, err := env.Service.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.Enabled {
		t.Fatal("expected peer to be disabled in store")
	}
}

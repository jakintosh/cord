package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIAddPeer_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add peer
	url := "/networks/testnet/peers"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5",
		"admin": false
	}`
	result := wire.TestPost[service.PeerInvite](env.Router, url, body)

	// verify result — handler returns a PeerInvite payload
	result.ExpectStatusOK(t, http.StatusCreated)
	if result.Data.Interface.NetworkName != "testnet" {
		t.Fatalf("network_name = %q, want testnet", result.Data.Interface.NetworkName)
	}
	if result.Data.Interface.PrivateKey == "" {
		t.Fatal("private_key should not be empty")
	}
	if result.Data.Interface.AssignedCidr == "" {
		t.Fatal("assigned_cidr should not be empty")
	}
	if result.Data.Server.PublicKey == "" {
		t.Fatal("server public_key should not be empty")
	}
	if result.Data.Server.ExternalEndpoint == "" {
		t.Fatal("server external_endpoint should not be empty")
	}
	if result.Data.Server.InternalEndpoint == "" {
		t.Fatal("server internal_endpoint should not be empty")
	}

	// verify invite was created
	invites, err := env.Service.ListInvites("testnet")
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(invites))
	}
	if invites[0].Name != "alice" {
		t.Fatalf("invite name = %q, want alice", invites[0].Name)
	}
}

func TestAPIAddPeer_AutoAssignIP(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add peer without explicit IP — should auto-assign
	url := "/networks/testnet/peers"
	body := `{
		"name": "bob",
		"admin": false
	}`
	result := wire.TestPost[service.PeerInvite](env.Router, url, body)

	// verify result
	result.ExpectStatusOK(t, http.StatusCreated)
	if result.Data.Interface.AssignedCidr == "" {
		t.Fatal("assigned_cidr should not be empty for auto-assigned IP")
	}
}

func TestAPIAddPeer_InvalidJSON(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// post garbage
	url := "/networks/testnet/peers"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIAddPeer_NetworkNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// add peer to nonexistent network
	url := "/networks/ghost/peers"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIAddPeer_DuplicateName(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add first peer
	url := "/networks/testnet/peers"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	result := wire.TestPost[service.PeerInvite](env.Router, url, body)
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
	result := wire.TestGet[[]api.PeerDTO](env.Router, url)

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
	if err := env.DB.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// list peers
	url := "/networks/testnet/peers"
	result := wire.TestGet[[]api.PeerDTO](env.Router, url)

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
			if p.Ip == "" {
				t.Fatal("ip should not be empty")
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
	if err := env.DB.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// rename peer
	url := "/networks/testnet/peers/alice"
	body := `{"name": "alicia"}`
	result := wire.TestPatch[api.PeerDTO](env.Router, url, body)

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
	if err := env.DB.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.0.5/32",
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
	if err := env.DB.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.0.5/32",
		Admin:     false,
		Enabled:   false,
		Confirmed: false,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// enable peer
	url := "/networks/testnet/peers/alice/enable"
	result := wire.TestPost[api.PeerDTO](env.Router, url, "")

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
	if err := env.DB.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	// disable peer
	url := "/networks/testnet/peers/alice/disable"
	result := wire.TestPost[api.PeerDTO](env.Router, url, "")

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

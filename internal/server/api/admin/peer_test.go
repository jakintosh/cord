package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIListPeers_Empty(
	t *testing.T,
) {
	// setup env and seed network (creates cord-server peer)
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list peers — should include the server peer
	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.Peer](env.Router, url)

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
	result := wire.TestGet[[]admin.Peer](env.Router, url)

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
	result := wire.TestPatch[admin.Peer](env.Router, url, body)

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

func TestAPIUpdatePeer_Empty(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/peers/cord-server"
	result := wire.TestPatch[any](env.Router, url, `{}`)

	result.ExpectStatusError(t, http.StatusBadRequest)
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
	url := "/networks/testnet/peers/alice"
	result := wire.TestPatch[admin.Peer](env.Router, url, `{"enabled":true}`)

	// verify result
	result.ExpectOK(t)

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
	url := "/networks/testnet/peers/alice"
	result := wire.TestPatch[admin.Peer](env.Router, url, `{"enabled":false}`)

	// verify result
	result.ExpectOK(t)

	// verify peer is disabled in store
	peer, err := env.Service.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.Enabled {
		t.Fatal("expected peer to be disabled in store")
	}
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
	result := wire.TestDelete[any](env.Router, url)

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

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

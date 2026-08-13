package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIListPeers_Empty(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.Peer](env.Router, url)

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
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.Peer](env.Router, url)

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
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	url := "/networks/testnet/peers/alice"
	body := `{"name": "alicia"}`
	result := wire.TestPatch[admin.Peer](env.Router, url, body)

	data := result.ExpectOK(t)
	if data.Name != "alicia" {
		t.Fatalf("name = %q, want alicia", data.Name)
	}

	_, err := env.Service.GetPeer("testnet", "alicia")
	if err != nil {
		t.Fatalf("get renamed peer: %v", err)
	}
}

func TestAPIRenamePeer_NotFound(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/peers/ghost"
	body := `{"name": "casper"}`
	result := wire.TestPatch[any](env.Router, url, body)

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
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, false, false)

	url := "/networks/testnet/peers/alice"
	result := wire.TestPatch[admin.Peer](env.Router, url, `{"enabled":true}`)

	result.ExpectOK(t)

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
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	url := "/networks/testnet/peers/alice"
	result := wire.TestPatch[admin.Peer](env.Router, url, `{"enabled":false}`)

	result.ExpectOK(t)

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
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	url := "/networks/testnet/peers/alice"
	result := wire.TestDelete[any](env.Router, url)

	result.ExpectOK(t)

	_, err := env.Service.GetPeer("testnet", "alice")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestAPIDeletePeer_NotFound(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/peers/ghost"
	result := wire.TestDelete[any](env.Router, url)

	result.ExpectStatusError(t, http.StatusNotFound)
}

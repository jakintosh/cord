package admin_test

import (
	"errors"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIListPeers_Empty(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.Peer](env.Router, url)

	// result
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
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	// request
	url := "/networks/testnet/peers"
	result := wire.TestGet[[]admin.Peer](env.Router, url)

	// result
	data := result.ExpectOK(t)
	if len(data) != 2 {
		t.Fatalf("expected 2 peers (cord-server + alice), got %d", len(data))
	}
	want := admin.Peer{Name: "alice", PublicKey: "alice-pub-key", Route: "10.0.0.5/32", Enabled: true}
	if data[0] != want || data[1].Name != "cord-server" {
		t.Fatalf("peers = %+v, want alice %+v followed by cord-server", data, want)
	}
}

func TestAPIRenamePeer_Success(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	// request
	url := "/networks/testnet/peers/alice"
	body := `{"name": "alicia"}`
	result := wire.TestPatch[admin.Peer](env.Router, url, body)

	// result
	data := result.ExpectOK(t)
	if data != (admin.Peer{Name: "alicia", PublicKey: "alice-pub-key", Route: "10.0.0.5/32", Enabled: true}) {
		t.Fatalf("name = %q, want alicia", data.Name)
	}

	if _, err := env.Service.GetPeer("testnet", "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("old name error = %v, want not found", err)
	}
	_, err := env.Service.GetPeer("testnet", "alicia")
	if err != nil {
		t.Fatalf("get renamed peer: %v", err)
	}
}

func TestAPIRenamePeer_NotFound(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/peers/ghost"
	body := `{"name": "casper"}`
	result := wire.TestPatch[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIUpdatePeer_Empty(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/peers/cord-server"
	body := "{}"
	result := wire.TestPatch[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIUpdatePeer_MalformedJSONPreservesPeer(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)
	before, err := env.Service.GetPeer("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}

	// request
	url := "/networks/testnet/peers/alice"
	body := `{"name":`
	result := wire.TestPatch[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusBadRequest)

	// state
	after, err := env.Service.GetPeer("testnet", "alice")
	if err != nil || *after != *before {
		t.Fatalf("peer = %+v, error = %v, want %+v", after, err, before)
	}
}

func TestAPIEnablePeer_Success(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, false, false)

	// request
	url := "/networks/testnet/peers/alice"
	body := `{
		"enabled": true
	}`
	result := wire.TestPatch[admin.Peer](env.Router, url, body)

	// result
	data := result.ExpectOK(t)
	want := admin.Peer{Name: "alice", PublicKey: "alice-pub-key", Route: "10.0.0.5/32", Enabled: true}
	if data != want {
		t.Fatalf("peer = %+v, want %+v", data, want)
	}

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
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	// request
	url := "/networks/testnet/peers/alice"
	body := `{
		"enabled": false
	}`
	result := wire.TestPatch[admin.Peer](env.Router, url, body)

	// result
	data := result.ExpectOK(t)
	want := admin.Peer{Name: "alice", PublicKey: "alice-pub-key", Route: "10.0.0.5/32", Enabled: false}
	if data != want {
		t.Fatalf("peer = %+v, want %+v", data, want)
	}

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
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	// request
	url := "/networks/testnet/peers/alice"
	result := wire.TestDelete[any](env.Router, url)

	// result
	result.ExpectOK(t)

	_, err := env.Service.GetPeer("testnet", "alice")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestAPIDeletePeer_NotFound(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/peers/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// result
	result.ExpectStatusError(t, http.StatusNotFound)
}

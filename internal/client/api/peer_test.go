package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestAPIListPeers_Empty(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// list peers
	url := "/networks/mynet/peers"
	result := wire.TestGet[[]api.Peer](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(data))
	}
}

func TestAPIListPeers_WithCachedData(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := env.Database.SetPeers("mynet", []service.Peer{{
		Name:      "alice",
		PublicKey: key,
		Route:     "10.42.0.9/32",
	}}); err != nil {
		t.Fatalf("seed peers: %v", err)
	}

	// list peers
	url := "/networks/mynet/peers"
	result := wire.TestGet[[]api.Peer](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(data))
	}
	if data[0].Name != "alice" {
		t.Fatalf("name = %q, want alice", data[0].Name)
	}
	if data[0].Route != "10.42.0.9/32" {
		t.Fatalf("route = %q, want 10.42.0.9/32", data[0].Route)
	}
	if data[0].Connected {
		t.Fatal("expected connected=false for a non-running network")
	}
	if data[0].LastHandshake != nil {
		t.Fatalf("last_handshake = %v, want nil", data[0].LastHandshake)
	}
}

func TestAPIListPeers_NetworkNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// list peers for nonexistent network
	url := "/networks/ghost/peers"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

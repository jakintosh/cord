package api_test

import (
	"net"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestAPIListPeers_Empty(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// send request
	url := "/networks/mynet/peers"
	result := wire.TestGet[[]api.Peer](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if len(data) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(data))
	}
}

func TestAPIListPeers_IncludesLiveHandshake(
	t *testing.T,
) {
	// setup cached identity and live device observation
	env := testutil.Setup(t)
	network := env.SeedEnabledNetwork(t, "mynet")
	key, err := wgtypes.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	testutil.SeedPeers(t, env.Database, "mynet", service.Peer{
		Name: "alice", PublicKey: key.String(), Route: "10.42.0.9/32",
	})
	env.Backend.Device(network.InterfaceName).SetPeers(wireguard.PeerStatus{
		PublicKey:     key,
		Endpoint:      &net.UDPAddr{IP: net.ParseIP("192.0.2.9"), Port: 51820},
		LastHandshake: testutil.FixedTime.Add(-time.Minute),
	})

	// send request
	url := "/networks/mynet/peers"
	result := wire.TestGet[[]api.Peer](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
	if len(data) != 1 {
		t.Fatalf("peers = %+v, want alice", data)
	}
	peer := data[0]
	if peer.Name != "alice" || peer.Route != "10.42.0.9/32" || peer.Endpoint != "192.0.2.9:51820" || !peer.Connected {
		t.Fatalf("live peer = %+v", peer)
	}
	wantHandshake := testutil.FixedTime.Add(-time.Minute).Format(time.RFC3339)
	if peer.LastHandshake == nil || *peer.LastHandshake != wantHandshake {
		t.Fatalf("handshake = %v, want %s", peer.LastHandshake, wantHandshake)
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
	testutil.SeedPeers(t, env.Database, "mynet", service.Peer{
		Name:      "alice",
		PublicKey: key,
		Route:     "10.42.0.9/32",
	})

	// send request
	url := "/networks/mynet/peers"
	result := wire.TestGet[[]api.Peer](env.Router, url)

	// verify response
	data := result.ExpectStatusOK(t, http.StatusOK)
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

	// send request
	url := "/networks/ghost/peers"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

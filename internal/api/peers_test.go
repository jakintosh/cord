package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

func TestAPIListPeers_ConfirmedPeerSeesServer(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// list peers as alice
	url := "/api/v1/peers"
	result := wire.TestGet[[]api.PublicPeerDTO](env.Router, url, testutil.FromAddr("10.0.0.10:40000"))

	// verify alice sees only the server peer
	peers := result.ExpectStatusOK(t, http.StatusOK)
	if len(peers) != 1 || peers[0].Name != "cord-server" {
		t.Fatalf("expected only cord-server, got %v", peers)
	}
}

func TestAPIListPeers_UnconfirmedPeerIsRejected(
	t *testing.T,
) {
	// setup env with a redeemed but unconfirmed peer
	env := testutil.SetupTestEnv(t)
	invite := testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)
	testutil.RedeemInvite(t, env, invite)

	// list peers from alice's assigned (but unconfirmed) address
	url := "/api/v1/peers"
	result := wire.TestGet[any](env.Router, url, testutil.FromAddr("10.0.0.10:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusUnauthorized)
}

func TestAPIReportEndpoints_AppearsInPeerList(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)
	from := testutil.FromAddr("10.0.0.10:40000")

	serverPeer, err := env.Service.GetPeer("cord-server")
	if err != nil {
		t.Fatalf("get server peer: %v", err)
	}

	// alice reports a sighting of the server
	url := "/api/v1/endpoint"
	body := fmt.Sprintf(`[
		{
			"peerKey": %q,
			"endpoint": "203.0.113.7:51820",
			"timestamp": %d
		}
	]`, serverPeer.PublicKey, time.Now().Unix())
	report := wire.TestPost[any](env.Router, url, body, from)

	// verify the report was accepted
	report.ExpectStatus(t, http.StatusOK)

	// verify the sighting shows up in alice's peer list, attributed
	// to alice as the witness
	list := wire.TestGet[[]api.PublicPeerDTO](env.Router, "/api/v1/peers", from)
	peers := list.ExpectStatusOK(t, http.StatusOK)
	if len(peers) != 1 || len(peers[0].Endpoints) != 1 {
		t.Fatalf("expected 1 peer with 1 endpoint, got %v", peers)
	}
	sighting := peers[0].Endpoints[0]
	if sighting.Endpoint != "203.0.113.7:51820" {
		t.Fatalf("unexpected endpoint %q", sighting.Endpoint)
	}
	alice, err := env.Service.GetPeer("alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if sighting.WitnessKey != alice.PublicKey {
		t.Fatalf("expected witness %q, got %q", alice.PublicKey, sighting.WitnessKey)
	}
}

func TestAPIReportEndpoints_RejectsMalformedBody(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// report with a non-array body
	url := "/api/v1/endpoint"
	body := `{"not": "a list"}`
	result := wire.TestPost[any](env.Router, url, body, testutil.FromAddr("10.0.0.10:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusBadRequest)
}

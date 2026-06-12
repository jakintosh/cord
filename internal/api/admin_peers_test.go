package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

var adminFrom = testutil.FromAddr(testutil.AdminAddr)

func TestAPICreatePeer_ReturnsInvitePayload(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// create a peer invite as admin
	url := "/api/v1/admin/peer"
	body := `{
		"name": "alice",
		"ip": "10.0.0.10"
	}`
	result := wire.TestPost[api.PeerInviteDTO](env.Router, url, body, adminFrom)

	// verify the invite payload is complete
	invite := result.ExpectStatusOK(t, http.StatusCreated)
	if invite.Interface.NetworkName != testutil.NetworkName {
		t.Fatalf("expected network %q, got %q", testutil.NetworkName, invite.Interface.NetworkName)
	}
	if invite.Interface.PrivateKey == "" || invite.Interface.AssignedCidr == "" {
		t.Fatalf("incomplete invite interface: %+v", invite.Interface)
	}
	if invite.Server.PublicKey == "" || invite.Server.ExternalEndpoint == "" {
		t.Fatalf("incomplete invite server info: %+v", invite.Server)
	}
}

func TestAPICreatePeer_RejectsDuplicateName(
	t *testing.T,
) {
	// setup env with an existing invite
	env := testutil.SetupTestEnv(t)
	testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)

	// create another invite with the same name
	url := "/api/v1/admin/peer"
	body := `{
		"name": "alice",
		"ip": "10.0.0.11"
	}`
	result := wire.TestPost[any](env.Router, url, body, adminFrom)

	// verify conflict
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPICreatePeer_RejectsInvalidIP(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// create an invite with a malformed ip
	url := "/api/v1/admin/peer"
	body := `{
		"name": "alice",
		"ip": "not-an-ip"
	}`
	result := wire.TestPost[any](env.Router, url, body, adminFrom)

	// verify rejection
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIAdminRoutes_RejectNonAdminPeer(
	t *testing.T,
) {
	// setup env with a joined non-admin peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// call an admin route as alice
	url := "/api/v1/admin/peers"
	result := wire.TestGet[any](env.Router, url, testutil.FromAddr("10.0.0.10:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusUnauthorized)
}

func TestAPIGetPeer_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// fetch a peer that does not exist
	url := "/api/v1/admin/peer/nobody"
	result := wire.TestGet[any](env.Router, url, adminFrom)

	// verify not found
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIListAdminPeers_IncludesUnconfirmed(
	t *testing.T,
) {
	// setup env with one redeemed (unconfirmed) peer
	env := testutil.SetupTestEnv(t)
	invite := testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)
	testutil.RedeemInvite(t, env, invite)

	// list all peers as admin
	url := "/api/v1/admin/peers"
	result := wire.TestGet[[]api.PeerDTO](env.Router, url, adminFrom)

	// verify the admin view includes the unconfirmed peer
	peers := result.ExpectStatusOK(t, http.StatusOK)
	if len(peers) != 2 {
		t.Fatalf("expected cord-server + alice, got %v", peers)
	}
	for _, peer := range peers {
		if peer.Name == "alice" && peer.Confirmed {
			t.Fatal("expected alice to be unconfirmed")
		}
	}
}

func TestAPIUpdatePeer_Rename(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// rename alice
	url := "/api/v1/admin/peer/alice"
	body := `{
		"name": "alice-renamed"
	}`
	result := wire.TestPatch[api.PeerDTO](env.Router, url, body, adminFrom)

	// verify the response and the service state
	peer := result.ExpectStatusOK(t, http.StatusOK)
	if peer.Name != "alice-renamed" {
		t.Fatalf("expected renamed peer, got %q", peer.Name)
	}
	if env.Service.CheckPeerExists("alice") {
		t.Fatal("old peer name should not exist after rename")
	}
}

func TestAPIUpdatePeer_DisableRevokesAccess(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// disable alice
	url := "/api/v1/admin/peer/alice"
	body := `{
		"enabled": false
	}`
	disable := wire.TestPatch[api.PeerDTO](env.Router, url, body, adminFrom)
	disable.ExpectStatus(t, http.StatusOK)

	// verify alice's API access is revoked
	list := wire.TestGet[any](env.Router, "/api/v1/peers", testutil.FromAddr("10.0.0.10:40000"))
	list.ExpectStatusError(t, http.StatusUnauthorized)
}

func TestAPIDeletePeer_RemovesPeer(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// delete alice
	url := "/api/v1/admin/peer/alice"
	result := wire.TestDelete[any](env.Router, url, adminFrom)

	// verify deletion
	result.ExpectStatus(t, http.StatusNoContent)
	if env.Service.CheckPeerExists("alice") {
		t.Fatal("expected peer to be gone after delete")
	}
}

func TestAPIDeletePeer_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// delete a peer that does not exist
	url := "/api/v1/admin/peer/nobody"
	result := wire.TestDelete[any](env.Router, url, adminFrom)

	// verify not found
	result.ExpectStatusError(t, http.StatusNotFound)
}

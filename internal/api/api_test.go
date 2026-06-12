package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

func TestAPIInviteRouter_ExposesOnlyRedemption(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// probe main-network routes on the invite router
	from := testutil.FromAddr("172.16.10.2:40000")
	peers := wire.TestGet[any](env.InviteRouter, "/api/v1/peers", from)
	admin := wire.TestGet[any](env.InviteRouter, "/api/v1/admin/peers", from)

	// verify both are unreachable
	peers.ExpectStatus(t, http.StatusNotFound)
	admin.ExpectStatus(t, http.StatusNotFound)
}

func TestAPIRouter_RejectsUnknownSource(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// list peers from an address with no peer record
	url := "/api/v1/peers"
	result := wire.TestGet[any](env.Router, url, testutil.FromAddr("10.0.99.99:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusUnauthorized)
}

func TestAPIMutationHook_FiresOncePerStateChange(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)
	if env.Mutations != 0 {
		t.Fatalf("expected no mutations before requests, got %d", env.Mutations)
	}

	// walk one peer through the join flow
	testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// verify invite creation, redemption, and confirmation each fired
	if env.Mutations != 3 {
		t.Fatalf("expected 3 mutations from join flow, got %d", env.Mutations)
	}
}

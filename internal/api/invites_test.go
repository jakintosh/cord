package api_test

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

func TestAPIJoinFlow_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// walk the full invite -> redeem -> confirm flow
	permKey := testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// verify the peer is confirmed in the service
	peer, err := env.Service.GetPeer("alice")
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	if peer.PublicKey != permKey {
		t.Fatalf("expected key %q, got %q", permKey, peer.PublicKey)
	}
	if !peer.Confirmed {
		t.Fatal("expected peer to be confirmed after join flow")
	}
}

func TestAPIRedeemInvite_ReturnsAssignment(
	t *testing.T,
) {
	// setup env and invite
	env := testutil.SetupTestEnv(t)
	invite := testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)

	// redeem from the invite's assigned address
	_, redeemed := testutil.RedeemInvite(t, env, invite)

	// verify the main-network assignment
	if redeemed.NetworkName != testutil.NetworkName {
		t.Fatalf("expected network %q, got %q", testutil.NetworkName, redeemed.NetworkName)
	}
	if redeemed.AssignedCidr != "10.0.0.10/16" {
		t.Fatalf("expected assignment 10.0.0.10/16, got %q", redeemed.AssignedCidr)
	}
	if redeemed.Server.ExternalEndpoint != "203.0.113.1:51820" {
		t.Fatalf("unexpected server endpoint %q", redeemed.Server.ExternalEndpoint)
	}
}

func TestAPIRedeemInvite_IsIdempotent(
	t *testing.T,
) {
	// setup env and a redeemed invite
	env := testutil.SetupTestEnv(t)
	invite := testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)
	from := testutil.FromAddr(testutil.InviteAddr(t, invite))
	permKey := testutil.NewPeerKey(t)

	// redeem twice with the same key: the network may have dropped the
	// first response, so the repeat must return the same configuration
	url := "/api/v1/invite/redeem"
	body := fmt.Sprintf(`{"publicKey": %q}`, permKey)
	first := wire.TestPost[api.RedeemResultDTO](env.InviteRouter, url, body, from)
	second := wire.TestPost[api.RedeemResultDTO](env.InviteRouter, url, body, from)

	// verify both succeed with identical payloads
	first.ExpectStatus(t, http.StatusOK)
	second.ExpectStatus(t, http.StatusOK)
	if !bytes.Equal(first.Raw, second.Raw) {
		t.Fatalf("repeat redeem differed:\n%s\n%s", first.Raw, second.Raw)
	}
}

func TestAPIRedeemInvite_RequiresKnownInviteAddress(
	t *testing.T,
) {
	// setup env with no invites
	env := testutil.SetupTestEnv(t)

	// redeem from an address holding no invite
	url := "/api/v1/invite/redeem"
	body := fmt.Sprintf(`{"publicKey": %q}`, testutil.NewPeerKey(t))
	result := wire.TestPost[any](env.InviteRouter, url, body, testutil.FromAddr("172.16.10.99:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusUnauthorized)
}

func TestAPIRedeemInvite_RequiresPublicKey(
	t *testing.T,
) {
	// setup env and invite
	env := testutil.SetupTestEnv(t)
	invite := testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)

	// redeem with an empty body
	url := "/api/v1/invite/redeem"
	body := "{}"
	result := wire.TestPost[any](env.InviteRouter, url, body, testutil.FromAddr(testutil.InviteAddr(t, invite)))

	// verify rejection
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIConfirmInvite_IsIdempotent(
	t *testing.T,
) {
	// setup env with a joined peer
	env := testutil.SetupTestEnv(t)
	permKey := testutil.JoinPeer(t, env, "alice", "10.0.0.10", false)

	// confirm again from the peer's address
	url := "/api/v1/invite/confirm"
	body := fmt.Sprintf(`{"publicKey": %q}`, permKey)
	result := wire.TestPost[any](env.Router, url, body, testutil.FromAddr("10.0.0.10:40000"))

	// verify the repeat succeeds
	result.ExpectStatus(t, http.StatusOK)
}

func TestAPIConfirmInvite_RequiresAssignedAddress(
	t *testing.T,
) {
	// setup env with a redeemed (unconfirmed) peer
	env := testutil.SetupTestEnv(t)
	invite := testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)
	permKey, _ := testutil.RedeemInvite(t, env, invite)

	// confirm from the wrong main-network address
	url := "/api/v1/invite/confirm"
	body := fmt.Sprintf(`{"publicKey": %q}`, permKey)
	result := wire.TestPost[any](env.Router, url, body, testutil.FromAddr("10.0.0.77:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusNotFound)
}

package api_test

import (
	"net/http"
	"strings"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

func TestAPIListInvites_Success(
	t *testing.T,
) {
	// setup env with one pending invite
	env := testutil.SetupTestEnv(t)
	testutil.CreateInvite(t, env, "alice", "10.0.0.10", true)

	// list invites
	url := "/api/v1/admin/invites"
	result := wire.TestGet[[]api.InviteDTO](env.Router, url, adminFrom)

	// verify the invite is reported
	invites := result.ExpectStatusOK(t, http.StatusOK)
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %v", invites)
	}
	invite := invites[0]
	if invite.Name != "alice" {
		t.Errorf("expected name 'alice', got '%s'", invite.Name)
	}
	if !strings.HasPrefix(invite.NetworkCidr, "10.0.0.10/") {
		t.Errorf("expected network cidr for 10.0.0.10, got '%s'", invite.NetworkCidr)
	}
	if !invite.Admin {
		t.Error("expected an admin invite")
	}
	if invite.Redeemed {
		t.Error("expected an unredeemed invite")
	}
	if invite.Expiration <= 0 {
		t.Errorf("expected a positive expiration, got %d", invite.Expiration)
	}
}

func TestAPIListInvites_ExcludesKeyMaterial(
	t *testing.T,
) {
	// setup env with one pending invite
	env := testutil.SetupTestEnv(t)
	testutil.CreateInvite(t, env, "alice", "10.0.0.10", false)

	// list invites as raw objects
	url := "/api/v1/admin/invites"
	result := wire.TestGet[[]map[string]any](env.Router, url, adminFrom)

	// verify no key fields travel on the wire
	invites := result.ExpectStatusOK(t, http.StatusOK)
	for _, invite := range invites {
		for field := range invite {
			if strings.Contains(strings.ToLower(field), "key") {
				t.Errorf("invite exposes key field '%s'", field)
			}
		}
	}
}

func TestAPIListInvites_RequiresAdmin(
	t *testing.T,
) {
	// setup env with a non-admin peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "bob", "10.0.0.20", false)

	// list invites from the non-admin peer's address
	url := "/api/v1/admin/invites"
	result := wire.TestGet[any](env.Router, url, testutil.FromAddr("10.0.0.20:40000"))

	// verify rejection
	result.ExpectStatusError(t, http.StatusUnauthorized)
}

func TestAPIListInvites_EmptyAfterConfirm(
	t *testing.T,
) {
	// setup env with a fully joined peer
	env := testutil.SetupTestEnv(t)
	testutil.JoinPeer(t, env, "bob", "10.0.0.20", false)

	// list invites
	url := "/api/v1/admin/invites"
	result := wire.TestGet[[]api.InviteDTO](env.Router, url, adminFrom)

	// verify confirmation consumed the invite
	invites := result.ExpectStatusOK(t, http.StatusOK)
	if len(invites) != 0 {
		t.Fatalf("expected no invites after confirmation, got %v", invites)
	}
}

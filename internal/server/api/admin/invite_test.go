package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIListInvites_Empty(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list invites
	url := "/networks/testnet/invites"
	result := wire.TestGet[[]admin.InviteDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 invites, got %d", len(data))
	}
}

func TestAPIListInvites_WithData(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add a peer which creates an invite
	url := "/networks/testnet/peers"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	result := wire.TestPost[any](env.Router, url, body)
	result.ExpectStatusOK(t, http.StatusCreated)

	// list invites
	url = "/networks/testnet/invites"
	listResult := wire.TestGet[[]admin.InviteDTO](env.Router, url)

	// verify result
	data := listResult.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(data))
	}
	if data[0].Name != "alice" {
		t.Fatalf("name = %q, want alice", data[0].Name)
	}
	if data[0].Redeemed {
		t.Fatal("invite should not be redeemed")
	}
	if data[0].ExpiresAt == "" {
		t.Fatal("expires_at should not be empty")
	}
}

func TestAPIListInvites_NetworkNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// list invites for nonexistent network — returns empty list
	url := "/networks/ghost/invites"
	result := wire.TestGet[[]admin.InviteDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 invites for nonexistent network, got %d", len(data))
	}
}

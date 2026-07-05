package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get status
	url := "/status"
	result := wire.TestGet[admin.StatusDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "ok" {
		t.Fatalf("status = %q, want ok", data.Status)
	}
	if len(data.Networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data.Networks))
	}
}

func TestAPIStatus_IncludesNetworks(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add a peer which creates a registration
	url := "/networks/testnet/registrations"
	body := `{
		"name": "alice",
		"ip": "10.0.0.5"
	}`
	createResult := wire.TestPost[any](env.Router, url, body)
	createResult.ExpectStatusOK(t, http.StatusCreated)

	// get status
	statusURL := "/status"
	result := wire.TestGet[admin.StatusDTO](env.Router, statusURL)

	// verify result
	data := result.ExpectOK(t)
	if len(data.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data.Networks))
	}
	net := data.Networks[0]
	if net.Name != "testnet" {
		t.Fatalf("name = %q, want testnet", net.Name)
	}
	if net.Enabled {
		t.Fatal("expected freshly created network to be disabled")
	}
	if net.PeerCount != 1 {
		t.Fatalf("peer_count = %d, want 1 (the bootstrapped server peer)", net.PeerCount)
	}
	if net.PendingRegistrationCount != 1 {
		t.Fatalf("pending_registration_count = %d, want 1", net.PendingRegistrationCount)
	}
}

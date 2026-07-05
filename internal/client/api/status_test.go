package api_test

import (
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get status
	url := "/status"
	result := wire.TestGet[api.StatusDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "ok" {
		t.Fatalf("status = %q, want ok", data.Status)
	}
	if data.Networks == nil {
		t.Fatal("expected networks to be a non-nil empty list")
	}
	if len(data.Networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(data.Networks))
	}
}

func TestAPIStatus_IncludesNetworks(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t, "mynet")

	// get status
	url := "/status"
	result := wire.TestGet[api.StatusDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data.Networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(data.Networks))
	}
	if data.Networks[0].Name != "mynet" {
		t.Fatalf("name = %q, want mynet", data.Networks[0].Name)
	}
}

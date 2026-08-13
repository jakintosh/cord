package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIListGroups_Empty(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/groups"
	result := wire.TestGet[[]admin.Group](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(data))
	}
}

func TestAPIListGroups_WithData(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := env.Service.CreateGroup("testnet", "marketing"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	url := "/networks/testnet/groups"
	result := wire.TestGet[[]admin.Group](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(data))
	}
}

func TestAPIPostGroup_Success(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/groups"
	body := `{"name": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusOK(t, http.StatusCreated)

	groups, err := env.Service.ListGroups("testnet")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "engineering" {
		t.Fatalf("group name = %q, want engineering", groups[0].Name)
	}
}

func TestAPIPostGroup_InvalidJSON(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/groups"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIPostGroup_Duplicate(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	url := "/networks/testnet/groups"
	body := `{"name": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIDeleteGroup_Success(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	url := "/networks/testnet/groups/engineering"
	result := wire.TestDelete[any](env.Router, url)

	result.ExpectOK(t)

	groups, err := env.Service.ListGroups("testnet")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups after delete, got %d", len(groups))
	}
}

func TestAPIDeleteGroup_NotFound(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/groups/nonexistent"
	result := wire.TestDelete[any](env.Router, url)

	result.ExpectStatusError(t, http.StatusNotFound)
}

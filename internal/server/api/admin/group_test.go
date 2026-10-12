package admin_test

import (
	"net/http"
	"reflect"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIListGroups_Empty(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/groups"
	result := wire.TestGet[[]admin.Group](env.Router, url)

	// result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(data))
	}
}

func TestAPIListGroups_WithData(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := env.Service.CreateGroup("testnet", "marketing"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	// request
	url := "/networks/testnet/groups"
	result := wire.TestGet[[]admin.Group](env.Router, url)

	// result
	data := result.ExpectOK(t)
	want := []admin.Group{{Name: "engineering"}, {Name: "marketing"}}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("groups = %+v, want %+v", data, want)
	}
}

func TestAPIPostGroup_Success(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/groups"
	body := `{"name": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	// result
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
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/groups"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIPostGroup_Duplicate(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	// request
	url := "/networks/testnet/groups"
	body := `{"name": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusConflict)
	groups, err := env.Service.ListGroups("testnet")
	if err != nil || len(groups) != 1 || groups[0].Name != "engineering" {
		t.Fatalf("groups after duplicate = %+v, %v", groups, err)
	}
}

func TestAPIPostGroup_MissingNetwork(
	t *testing.T,
) {
	// Setup an empty real database and running API.
	env := testutil.Setup(t)

	// Request a group under an absent parent.
	url := "/networks/missing/groups"
	body := `{"name": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	// Verify the stable error and unchanged storage.
	apiErr := result.ExpectStatusError(t, http.StatusNotFound)
	if apiErr.Message != "resource not found" {
		t.Fatalf("error = %q", apiErr.Message)
	}
	groups, err := env.Service.ListGroups("missing")
	if err != nil || len(groups) != 0 {
		t.Fatalf("groups after rejection = %+v, %v", groups, err)
	}
}

func TestAPIDeleteGroup_Success(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	if _, err := env.Service.CreateGroup("testnet", "engineering"); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	// request
	url := "/networks/testnet/groups/engineering"
	result := wire.TestDelete[any](env.Router, url)

	// result
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
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// request
	url := "/networks/testnet/groups/nonexistent"
	result := wire.TestDelete[any](env.Router, url)

	// result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func seedGroup(
	t *testing.T,
	env *testutil.APIEnv,
	name string,
) {
	t.Helper()
	if _, err := env.Service.CreateGroup("testnet", name); err != nil {
		t.Fatalf("seed group %s: %v", name, err)
	}
}

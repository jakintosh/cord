package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedGroup(t *testing.T, env *testutil.APIEnv, name string) {
	t.Helper()
	if _, err := env.Service.CreateGroup("testnet", name); err != nil {
		t.Fatalf("seed group %s: %v", name, err)
	}
}

func TestAPIAddAssociation_Success(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedGroup(t, env, "engineering")
	seedGroup(t, env, "marketing")

	url := "/networks/testnet/associations"
	body := `{
		"group1": "engineering",
		"group2": "marketing"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusOK(t, http.StatusCreated)

	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list associations: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association, got %d", len(assocs))
	}
}

func TestAPIAddAssociation_InvalidJSON(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/associations"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIAddAssociation_SelfAssociate(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedGroup(t, env, "engineering")

	url := "/networks/testnet/associations"
	body := `{
		"group1": "engineering",
		"group2": "engineering"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusOK(t, http.StatusCreated)
}

func TestAPIListAssociations_Empty(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/associations"
	result := wire.TestGet[[]admin.Association](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 associations, got %d", len(data))
	}
}

func TestAPIListAssociations_WithData(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedGroup(t, env, "engineering")
	seedGroup(t, env, "marketing")
	if err := env.Service.CreateAssociation("testnet", "engineering", "marketing"); err != nil {
		t.Fatalf("seed association: %v", err)
	}

	url := "/networks/testnet/associations"
	result := wire.TestGet[[]admin.Association](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 association, got %d", len(data))
	}
	if data[0].Group1 != "engineering" || data[0].Group2 != "marketing" {
		t.Fatalf("association = %q/%q, want engineering/marketing", data[0].Group1, data[0].Group2)
	}
}

func TestAPIDeleteAssociation_Success(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedGroup(t, env, "engineering")
	seedGroup(t, env, "marketing")
	if err := env.Service.CreateAssociation("testnet", "engineering", "marketing"); err != nil {
		t.Fatalf("seed association: %v", err)
	}

	url := "/networks/testnet/associations/delete"
	body := `{
		"group1": "engineering",
		"group2": "marketing"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectOK(t)

	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list associations: %v", err)
	}
	if len(assocs) != 0 {
		t.Fatalf("expected 0 associations after delete, got %d", len(assocs))
	}
}

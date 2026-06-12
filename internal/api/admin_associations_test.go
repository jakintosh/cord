package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

// seedCidrPair creates two child CIDRs ready for association tests
func seedCidrPair(
	t *testing.T,
	env *testutil.TestEnv,
) {
	t.Helper()

	for _, body := range []string{
		`{"name": "infra", "cidr": "10.0.1.0/24"}`,
		`{"name": "fleet", "cidr": "10.0.2.0/24"}`,
	} {
		result := wire.TestPost[api.CidrDTO](env.Router, "/api/v1/admin/cidr", body, adminFrom)
		result.ExpectStatus(t, http.StatusCreated)
	}
}

func TestAPICreateAssociation_Success(
	t *testing.T,
) {
	// setup env with two cidrs
	env := testutil.SetupTestEnv(t)
	seedCidrPair(t, env)

	// associate them
	url := "/api/v1/admin/association"
	body := `{
		"cidr1": "infra",
		"cidr2": "fleet"
	}`
	result := wire.TestPost[api.AssociationDTO](env.Router, url, body, adminFrom)

	// verify creation and listing
	result.ExpectStatus(t, http.StatusCreated)
	listed := wire.TestGet[[]api.AssociationDTO](env.Router, "/api/v1/admin/associations", adminFrom)
	associations := listed.ExpectStatusOK(t, http.StatusOK)
	if len(associations) != 1 {
		t.Fatalf("expected 1 association, got %v", associations)
	}
}

func TestAPICreateAssociation_RejectsUnknownCidr(
	t *testing.T,
) {
	// setup env with no child cidrs
	env := testutil.SetupTestEnv(t)

	// associate names that do not exist
	url := "/api/v1/admin/association"
	body := `{
		"cidr1": "ghost",
		"cidr2": "phantom"
	}`
	result := wire.TestPost[any](env.Router, url, body, adminFrom)

	// verify rejection
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPICreateAssociation_RequiresBothCidrs(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// associate with a missing field
	url := "/api/v1/admin/association"
	body := `{
		"cidr1": "infra"
	}`
	result := wire.TestPost[any](env.Router, url, body, adminFrom)

	// verify rejection
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIDeleteAssociation_Success(
	t *testing.T,
) {
	// setup env with an association
	env := testutil.SetupTestEnv(t)
	seedCidrPair(t, env)
	created := wire.TestPost[api.AssociationDTO](env.Router, "/api/v1/admin/association", `{
		"cidr1": "infra",
		"cidr2": "fleet"
	}`, adminFrom)
	created.ExpectStatus(t, http.StatusCreated)

	// delete it (order reversed: associations are symmetric)
	url := "/api/v1/admin/association/fleet/infra"
	result := wire.TestDelete[any](env.Router, url, adminFrom)

	// verify deletion
	result.ExpectStatus(t, http.StatusNoContent)
	listed := wire.TestGet[[]api.AssociationDTO](env.Router, "/api/v1/admin/associations", adminFrom)
	associations := listed.ExpectStatusOK(t, http.StatusOK)
	if len(associations) != 0 {
		t.Fatalf("expected no associations after delete, got %v", associations)
	}
}

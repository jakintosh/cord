package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/testutil"
)

func TestAPICreateCidr_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// create a child cidr within the root range
	url := "/api/v1/admin/cidr"
	body := `{
		"name": "infra",
		"cidr": "10.0.1.0/24"
	}`
	result := wire.TestPost[api.CidrDTO](env.Router, url, body, adminFrom)

	// verify the created record
	cidr := result.ExpectStatusOK(t, http.StatusCreated)
	if cidr.Name != "infra" || cidr.Cidr != "10.0.1.0/24" {
		t.Fatalf("unexpected cidr %+v", cidr)
	}
}

func TestAPICreateCidr_RejectsOutsideRootRange(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// create a cidr outside the 10.0.0.0/16 root
	url := "/api/v1/admin/cidr"
	body := `{
		"name": "outside",
		"cidr": "192.168.0.0/24"
	}`
	result := wire.TestPost[any](env.Router, url, body, adminFrom)

	// verify rejection as invalid input
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPICreateCidr_RejectsDuplicateName(
	t *testing.T,
) {
	// setup env with an existing cidr
	env := testutil.SetupTestEnv(t)
	first := wire.TestPost[api.CidrDTO](env.Router, "/api/v1/admin/cidr", `{
		"name": "infra",
		"cidr": "10.0.1.0/24"
	}`, adminFrom)
	first.ExpectStatus(t, http.StatusCreated)

	// create another cidr with the same name
	url := "/api/v1/admin/cidr"
	body := `{
		"name": "infra",
		"cidr": "10.0.2.0/24"
	}`
	result := wire.TestPost[any](env.Router, url, body, adminFrom)

	// verify conflict
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIDeleteCidr_RejectsRootCidr(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// the root cidr shares the network's name
	url := "/api/v1/admin/cidr/" + testutil.NetworkName
	result := wire.TestDelete[any](env.Router, url, adminFrom)

	// verify the root cidr is protected
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIDeleteCidr_RemovesAssociations(
	t *testing.T,
) {
	// setup env with two associated cidrs
	env := testutil.SetupTestEnv(t)
	for _, body := range []string{
		`{"name": "infra", "cidr": "10.0.1.0/24"}`,
		`{"name": "fleet", "cidr": "10.0.2.0/24"}`,
	} {
		created := wire.TestPost[api.CidrDTO](env.Router, "/api/v1/admin/cidr", body, adminFrom)
		created.ExpectStatus(t, http.StatusCreated)
	}
	associated := wire.TestPost[api.AssociationDTO](env.Router, "/api/v1/admin/association", `{
		"cidr1": "infra",
		"cidr2": "fleet"
	}`, adminFrom)
	associated.ExpectStatus(t, http.StatusCreated)

	// delete one side of the association
	url := "/api/v1/admin/cidr/infra"
	result := wire.TestDelete[any](env.Router, url, adminFrom)

	// verify the cidr and its association are gone
	result.ExpectStatus(t, http.StatusNoContent)
	listed := wire.TestGet[[]api.AssociationDTO](env.Router, "/api/v1/admin/associations", adminFrom)
	associations := listed.ExpectStatusOK(t, http.StatusOK)
	if len(associations) != 0 {
		t.Fatalf("expected no associations after cidr delete, got %v", associations)
	}
}

func TestAPIGetCidr_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.SetupTestEnv(t)

	// fetch a cidr that does not exist
	url := "/api/v1/admin/cidr/nothing"
	result := wire.TestGet[any](env.Router, url, adminFrom)

	// verify not found
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIRenameCidr_Success(
	t *testing.T,
) {
	// setup env with an existing cidr
	env := testutil.SetupTestEnv(t)
	created := wire.TestPost[api.CidrDTO](env.Router, "/api/v1/admin/cidr", `{
		"name": "infra",
		"cidr": "10.0.1.0/24"
	}`, adminFrom)
	created.ExpectStatus(t, http.StatusCreated)

	// rename it
	url := "/api/v1/admin/cidr/infra"
	body := `{
		"name": "infra-renamed"
	}`
	result := wire.TestPatch[api.CidrDTO](env.Router, url, body, adminFrom)

	// verify the renamed record
	cidr := result.ExpectStatusOK(t, http.StatusOK)
	if cidr.Name != "infra-renamed" {
		t.Fatalf("expected renamed cidr, got %q", cidr.Name)
	}
}

func TestAPIDeleteCidr_Success(
	t *testing.T,
) {
	// setup env with an existing cidr
	env := testutil.SetupTestEnv(t)
	created := wire.TestPost[api.CidrDTO](env.Router, "/api/v1/admin/cidr", `{
		"name": "infra",
		"cidr": "10.0.1.0/24"
	}`, adminFrom)
	created.ExpectStatus(t, http.StatusCreated)

	// delete it
	url := "/api/v1/admin/cidr/infra"
	result := wire.TestDelete[any](env.Router, url, adminFrom)

	// verify deletion
	result.ExpectStatus(t, http.StatusNoContent)
	listed := wire.TestGet[[]api.CidrDTO](env.Router, "/api/v1/admin/cidrs", adminFrom)
	cidrs := listed.ExpectStatusOK(t, http.StatusOK)
	if len(cidrs) != 1 {
		t.Fatalf("expected only the root cidr to remain, got %v", cidrs)
	}
}

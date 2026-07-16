package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func seedCidr(t *testing.T, env *testutil.APIEnv, name string, cidr string) {
	t.Helper()
	if err := env.Service.CreateCidr("testnet", name, cidr); err != nil {
		t.Fatalf("seed cidr %s: %v", name, err)
	}
}

func TestAPIListAssignments_Empty(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/assignments"
	result := wire.TestGet[[]admin.Assignment](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 assignments, got %d", len(data))
	}
}

func TestAPIListAssignments_WithData(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedCidr(t, env, "servers", "10.0.0.0/24")
	seedGroup(t, env, "engineering")
	if err := env.Service.AssignGroup("testnet", "servers", "engineering"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	url := "/networks/testnet/assignments"
	result := wire.TestGet[[]admin.Assignment](env.Router, url)

	data := result.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(data))
	}
	if data[0].CidrName != "servers" || data[0].GroupName != "engineering" {
		t.Fatalf("assignment = %q/%q, want servers/engineering", data[0].CidrName, data[0].GroupName)
	}
}

func TestAPIAddAssignment_Success(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedCidr(t, env, "servers", "10.0.0.0/24")
	seedGroup(t, env, "engineering")

	url := "/networks/testnet/assignments"
	body := `{"cidr": "servers", "group": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusOK(t, http.StatusCreated)

	assignments, err := env.Service.ListAssignments("testnet")
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
}

func TestAPIAddAssignment_InvalidJSON(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/assignments"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIAddAssignment_Duplicate(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedCidr(t, env, "servers", "10.0.0.0/24")
	seedGroup(t, env, "engineering")
	if err := env.Service.AssignGroup("testnet", "servers", "engineering"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	url := "/networks/testnet/assignments"
	body := `{"cidr": "servers", "group": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIRemoveAssignment_Success(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedCidr(t, env, "servers", "10.0.0.0/24")
	seedGroup(t, env, "engineering")
	if err := env.Service.AssignGroup("testnet", "servers", "engineering"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	url := "/networks/testnet/assignments/delete"
	body := `{"cidr": "servers", "group": "engineering"}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectOK(t)

	assignments, err := env.Service.ListAssignments("testnet")
	if err != nil {
		t.Fatalf("list assignments: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected 0 assignments after remove, got %d", len(assignments))
	}
}

func TestAPIRemoveAssignment_Idempotent(
	t *testing.T,
) {
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	url := "/networks/testnet/assignments/delete"
	body := `{"cidr": "nonexistent", "group": "nonexistent"}`
	result := wire.TestPost[any](env.Router, url, body)

	result.ExpectOK(t)
}

package admin_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIError_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get nonexistent network
	url := "/networks/ghost"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	apiErr := result.ExpectStatusError(t, http.StatusNotFound)
	if apiErr.Message == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestAPIError_Conflict(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// post duplicate network
	url := "/networks"
	body := `{
		"name": "testnet",
		"external_ip": "1.2.3.4",
		"main_cidr": "10.42.0.0/16",
		"main_wg_port": 51820
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	apiErr := result.ExpectStatusError(t, http.StatusConflict)
	if apiErr.Message == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestAPIError_BadRequest(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// post malformed json
	url := "/networks"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	apiErr := result.ExpectStatusError(t, http.StatusBadRequest)
	if apiErr.Message == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestAPIError_HidesUnexpectedDiagnosticsAndLogsContext(
	t *testing.T,
) {
	// Setup a real runtime whose store fails on an admin operation.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	var logs bytes.Buffer
	api, err := admin.New(admin.Options{
		Runtime: env.Runtime,
		Logger:  slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}

	// Request an operation that reaches the failed store.
	url := "/networks/testnet/groups"
	result := wire.TestGet[[]admin.Group](api.Router(), url)

	// Public bodies stay generic; server logs retain operation and diagnostics.
	apiErr := result.ExpectStatusError(t, http.StatusInternalServerError)
	if apiErr.Message != "internal server error" || bytes.Contains(result.Raw, []byte("database")) {
		t.Fatalf("public error = %s", result.Raw)
	}
	for _, want := range []string{"database is closed", "operation=\"GET /networks/{name}/groups\"", "network=testnet"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log missing %q: %s", want, logs.String())
		}
	}
}

package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
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

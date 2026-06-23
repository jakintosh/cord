package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPIRouter_ExposesNetworkRoutes(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// list networks on empty router — should return 200 with empty data
	result := wire.TestGet[[]api.NetworkDTO](env.Router, "/networks")
	result.ExpectOK(t)

	// get nonexistent network should return 404
	notFound := wire.TestGet[any](env.Router, "/networks/ghost")
	notFound.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIError_NotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get nonexistent path
	url := "/nonexistent"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	result.ExpectStatus(t, http.StatusNotFound)
}

package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPIError_RuntimeStoppedReturnsUnavailable(
	t *testing.T,
) {
	// setup stopped runtime
	env := testutil.SetupRuntime(t)
	control, err := api.New(api.Options{Runtime: env.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Stop(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/status", "/networks", "/networks/missing"} {
		t.Run(path, func(t *testing.T) {
			// send request
			url := path
			result := wire.TestGet[any](control.Router(), url)

			// verify response
			result.ExpectStatusError(t, http.StatusServiceUnavailable)
		})
	}
}

func TestAPIError_StorageFailureReturnsInternalServerError(
	t *testing.T,
) {
	// setup a real storage failure without a substitute store
	env := testutil.Setup(t)
	if err := env.Database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	// send request
	url := "/networks"
	result := wire.TestGet[any](env.Router, url)

	// verify the failure remains a server error rather than an empty result
	result.ExpectStatusError(t, http.StatusInternalServerError)
}

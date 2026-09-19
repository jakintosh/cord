package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPINew_RequiresRuntime(
	t *testing.T,
) {
	control, err := api.New(api.Options{})
	if err == nil || control != nil {
		t.Fatalf("New without runtime = %v, %v; want nil and error", control, err)
	}
}

func TestAPIRouter_UnknownPathReturnsNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// send request
	url := "/nonexistent"
	result := wire.TestGet[any](env.Router, url)

	// verify result
	result.ExpectStatus(t, http.StatusNotFound)
}

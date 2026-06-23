package api_test

import (
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestAPIStatus_Success(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// get status
	url := "/status"
	result := wire.TestGet[api.StatusResponse](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "ok" {
		t.Fatalf("status = %q, want ok", data.Status)
	}
}

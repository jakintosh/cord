package api_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
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

func TestAPIRouter_ExposesNetworkRoutes(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// list networks on empty router – should return 200 with empty data
	result := wire.TestGet[[]string](env.Router, "/networks")
	result.ExpectOK(t)

	// get nonexistent network should return 404
	notFound := wire.TestGet[any](env.Router, "/networks/ghost")
	notFound.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIRouter_ExposesPeerRoutes(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list peers on empty network should return 200
	result := wire.TestGet[[]api.PeerDTO](env.Router, "/networks/testnet/peers")
	result.ExpectOK(t)
}

func TestAPIRouter_ExposesCidrRoutes(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list cidrs on empty network should return 200
	result := wire.TestGet[[]api.CidrDTO](env.Router, "/networks/testnet/cidrs")
	result.ExpectOK(t)
}

func TestAPIRouter_ExposesAssociationRoutes(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list associations on empty network should return 200
	result := wire.TestGet[[]api.AssociationDTO](env.Router, "/networks/testnet/associations")
	result.ExpectOK(t)
}

func TestAPIRouter_ExposesInviteRoutes(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list invites on empty network should return 200
	result := wire.TestGet[[]api.InviteDTO](env.Router, "/networks/testnet/invites")
	result.ExpectOK(t)
}

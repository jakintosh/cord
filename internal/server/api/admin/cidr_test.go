package admin_test

import (
	"errors"
	"net"
	"net/http"
	"reflect"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIAddCidr_Success(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add cidr
	url := "/networks/testnet/cidrs"
	body := `{
		"name": "engineering",
		"cidr": "10.0.1.0/24"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result — status-only mutation, no response body
	result.ExpectStatusOK(t, http.StatusCreated)

	// verify cidr exists in store
	c, err := env.Service.GetCidr("testnet", "engineering")
	if err != nil {
		t.Fatalf("get cidr from service: %v", err)
	}
	if c.Cidr != "10.0.1.0/24" {
		t.Fatalf("cidr in store = %q, want 10.0.1.0/24", c.Cidr)
	}
}

func TestAPIAddCidr_InvalidJSON(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// post garbage
	url := "/networks/testnet/cidrs"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIAddCidr_NetworkNotFound(
	t *testing.T,
) {
	// setup env
	env := testutil.Setup(t)

	// add cidr to nonexistent network
	url := "/networks/ghost/cidrs"
	body := `{
		"name": "engineering",
		"cidr": "10.0.1.0/24"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIAddCidr_OutsideMainCidr(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add cidr outside root
	url := "/networks/testnet/cidrs"
	body := `{
		"name": "outside",
		"cidr": "192.168.1.0/24"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIListCidrs_Empty(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list cidrs
	url := "/networks/testnet/cidrs"
	result := wire.TestGet[[]admin.Cidr](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 2 {
		t.Fatalf("expected 2 cidrs (root + server), got %d", len(data))
	}
}

func TestAPIListCidrs_WithData(
	t *testing.T,
) {
	// setup env and seed network + cidr
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")

	// list cidrs
	url := "/networks/testnet/cidrs"
	result := wire.TestGet[[]admin.Cidr](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 3 {
		t.Fatalf("expected 3 cidrs (root + server + engineering), got %d", len(data))
	}
	want := []admin.Cidr{{Name: "cord-server", Cidr: "10.0.0.1/32"}, {Name: "engineering", Cidr: "10.0.1.0/24"}, {Name: "testnet", Cidr: "10.0.0.0/16"}}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("cidrs = %+v, want %+v", data, want)
	}
}

func TestAPIRenameCidr_Success(
	t *testing.T,
) {
	// setup env and seed network + cidr
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")

	// rename cidr
	url := "/networks/testnet/cidrs/engineering"
	body := `{"name": "eng"}`
	result := wire.TestPatch[any](env.Router, url, body)

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	if _, err := env.Service.GetCidr("testnet", "engineering"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("old name error = %v, want not found", err)
	}

	// verify cidr was renamed in store
	c, err := env.Service.GetCidr("testnet", "eng")
	if err != nil {
		t.Fatalf("get renamed cidr: %v", err)
	}
	if c.Cidr != "10.0.1.0/24" {
		t.Fatalf("cidr = %q, want 10.0.1.0/24", c.Cidr)
	}
}

func TestAPIRenameCidr_NotFound(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// rename nonexistent cidr
	url := "/networks/testnet/cidrs/ghost"
	body := `{"name": "casper"}`
	result := wire.TestPatch[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPIDeleteCidr_Success(
	t *testing.T,
) {
	// setup env and seed network + cidr
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")

	// delete cidr
	url := "/networks/testnet/cidrs/engineering"
	result := wire.TestDelete[any](env.Router, url)

	// verify result — status-only mutation, no response body
	result.ExpectOK(t)

	// verify cidr is gone
	_, err := env.Service.GetCidr("testnet", "engineering")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestAPIDeleteCidr_NotFound(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// delete nonexistent cidr
	url := "/networks/testnet/cidrs/ghost"
	result := wire.TestDelete[any](env.Router, url)

	// verify result
	result.ExpectStatusError(t, http.StatusNotFound)
}

func TestAPICidrGroups_AddListRemove(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "servers", "10.0.1.0/24")
	seedGroup(t, env, "engineering")

	// request
	url := "/networks/testnet/cidrs/servers/groups"
	body := `{"group":"engineering"}`
	wire.TestPost[any](env.Router, url, body).ExpectStatusOK(t, http.StatusCreated)

	groups := wire.TestGet[[]admin.Group](env.Router, url).ExpectOK(t)
	if len(groups) != 1 || groups[0].Name != "engineering" {
		t.Fatalf("groups = %+v, want engineering", groups)
	}

	wire.TestPost[any](env.Router, url, body).ExpectStatusError(t, http.StatusConflict)
	wire.TestDelete[any](env.Router, url+"/engineering").ExpectOK(t)
	groups = wire.TestGet[[]admin.Group](env.Router, url).ExpectOK(t)
	if len(groups) != 0 {
		t.Fatalf("groups after removal = %+v, want empty", groups)
	}
}

func TestAPICidrGroups_MissingCidr(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedGroup(t, env, "engineering")

	missingURL := "/networks/testnet/cidrs/missing/groups"
	body := `{
		"group": "engineering"
	}`
	wire.TestGet[[]admin.Group](env.Router, missingURL).ExpectStatusError(t, http.StatusNotFound)
	wire.TestPost[any](env.Router, missingURL, body).ExpectStatusError(t, http.StatusNotFound)
	wire.TestDelete[any](env.Router, missingURL+"/engineering").ExpectStatusError(t, http.StatusNotFound)

}

func TestAPICidrGroups_RejectsProvisionalPeerCidr(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	seedGroup(t, env, "engineering")

	if _, err := env.Service.CreateRegistration(
		"testnet",
		"alice",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	registrations, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if _, err := env.Service.RedeemRegistration(
		"testnet",
		registrations[0].InvitePublicKey,
		"alice-key",
	); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}

	// request
	url := "/networks/testnet/cidrs/alice/groups"
	body := `{
		"group": "engineering"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusConflict)
}

func TestAPIAddCidrGroup_MalformedJSON(
	t *testing.T,
) {
	// setup
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "servers", "10.0.1.0/24")

	// request
	url := "/networks/testnet/cidrs/servers/groups"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// result
	result.ExpectStatusError(t, http.StatusBadRequest)

	// state
	groups, err := env.Service.ListCidrGroups("testnet", "servers")
	if err != nil || len(groups) != 0 {
		t.Fatalf("groups = %+v, error = %v, want empty", groups, err)
	}
}

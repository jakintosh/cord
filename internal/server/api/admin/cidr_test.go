package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
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

func TestAPICreateNetwork_OverlappingCidrs(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// add overlapping invite CIDR at network creation time
	url := "/networks"
	body := `{
		"name": "overlap",
		"cidr": "10.0.0.0/16",
		"invite_cidr": "10.0.1.0/24",
		"external_ip": "1.2.3.4",
		"port": 51820
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
	foundEng := false
	for _, c := range data {
		if c.Name == "engineering" {
			foundEng = true
			break
		}
	}
	if !foundEng {
		t.Fatal("engineering cidr not found in list")
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

package admin_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIAddAssociation_Success(
	t *testing.T,
) {
	// setup env and seed network + cidrs
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")
	env.SeedCIDR(t, "testnet", "marketing", "10.0.2.0/24")

	// add association
	url := "/networks/testnet/associations"
	body := `{
		"cidr1": "engineering",
		"cidr2": "marketing"
	}`
	result := wire.TestPost[admin.AssociationDTO](env.Router, url, body)

	// verify result
	data := result.ExpectStatusOK(t, http.StatusCreated)
	if data.Cidr1 != "engineering" {
		t.Fatalf("cidr1 = %q, want engineering", data.Cidr1)
	}
	if data.Cidr2 != "marketing" {
		t.Fatalf("cidr2 = %q, want marketing", data.Cidr2)
	}

	// verify association exists
	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list associations: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association, got %d", len(assocs))
	}
}

func TestAPIAddAssociation_InvalidJSON(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// post garbage
	url := "/networks/testnet/associations"
	body := `{`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIAddAssociation_SelfAssociate(
	t *testing.T,
) {
	// setup env and seed network + cidrs
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")

	// associate with itself
	url := "/networks/testnet/associations"
	body := `{
		"cidr1": "engineering",
		"cidr2": "engineering"
	}`
	result := wire.TestPost[any](env.Router, url, body)

	// verify result
	result.ExpectStatusError(t, http.StatusBadRequest)
}

func TestAPIListAssociations_Empty(
	t *testing.T,
) {
	// setup env and seed network
	env := testutil.Setup(t)
	env.SeedNetwork(t)

	// list associations
	url := "/networks/testnet/associations"
	result := wire.TestGet[[]admin.AssociationDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 0 {
		t.Fatalf("expected 0 associations, got %d", len(data))
	}
}

func TestAPIListAssociations_WithData(
	t *testing.T,
) {
	// setup env and seed network + cidrs + association
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")
	env.SeedCIDR(t, "testnet", "marketing", "10.0.2.0/24")
	if err := env.Service.CreateAssociation("testnet", "engineering", "marketing"); err != nil {
		t.Fatalf("seed association: %v", err)
	}

	// list associations
	url := "/networks/testnet/associations"
	result := wire.TestGet[[]admin.AssociationDTO](env.Router, url)

	// verify result
	data := result.ExpectOK(t)
	if len(data) != 1 {
		t.Fatalf("expected 1 association, got %d", len(data))
	}
	// stored normalized so cidr1 < cidr2
	if data[0].Cidr1 != "engineering" || data[0].Cidr2 != "marketing" {
		t.Fatalf("association = %q/%q, want engineering/marketing", data[0].Cidr1, data[0].Cidr2)
	}
}

func TestAPIDeleteAssociation_Success(
	t *testing.T,
) {
	// setup env and seed network + cidrs + association
	env := testutil.Setup(t)
	env.SeedNetwork(t)
	env.SeedCIDR(t, "testnet", "engineering", "10.0.1.0/24")
	env.SeedCIDR(t, "testnet", "marketing", "10.0.2.0/24")
	if err := env.Service.CreateAssociation("testnet", "engineering", "marketing"); err != nil {
		t.Fatalf("seed association: %v", err)
	}

	// delete association
	url := "/networks/testnet/associations/delete"
	body := `{
		"cidr1": "engineering",
		"cidr2": "marketing"
	}`
	result := wire.TestPost[api.DeleteResponse](env.Router, url, body)

	// verify result
	data := result.ExpectOK(t)
	if data.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", data.Status)
	}

	// verify association is gone
	assocs, err := env.Service.ListAssociations("testnet")
	if err != nil {
		t.Fatalf("list associations: %v", err)
	}
	if len(assocs) != 0 {
		t.Fatalf("expected 0 associations after delete, got %d", len(assocs))
	}
}

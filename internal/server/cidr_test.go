package server_test

import (
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestDeleteCidr(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	if err := addAndRedeemPeer(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if err := addAndRedeemPeer(ctx, testUser2); err != nil {
		t.Fatal(err)
	}
	if err := addAndRedeemPeer(ctx, testUser3); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 2); err != nil {
		t.Fatal(err)
	}

	if err := ctx.DeleteCidr(fleetCidr.Name); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 3); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAndRenameCidr(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	// Create a new child CIDR within the root range
	if err := ctx.CreateCidr(server.CreateCidrRequest{
		Name: "extra",
		Cidr: "10.0.64.0/18",
	}); err != nil {
		t.Fatalf("expected create cidr to succeed: %v", err)
	}

	// Renaming the CIDR should succeed and be usable in associations
	if err := ctx.RenameCidr(
		"extra",
		server.UpdateCidrRequest{
			Name: "extra-renamed",
		},
	); err != nil {
		t.Fatalf("failed to rename cidr: %v", err)
	}

	// Using the renamed CIDR in an association should work
	if err := ctx.CreateAssociation("extra-renamed", "infra"); err != nil {
		t.Fatalf("failed to create association with renamed cidr: %v", err)
	}
}

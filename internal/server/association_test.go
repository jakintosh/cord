package server_test

import "testing"

func TestPeerAssociations(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	if err := addAndRedeemPeer(ctx, testServer); err != nil {
		t.Fatal(err)
	}

	if err := addAndRedeemPeer(ctx, testUser); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 0); err != nil {
		t.Fatal(err)
	}

	if err := ctx.CreateAssociation("fleet", "infra"); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 2); err != nil {
		t.Fatal(err)
	}

	if err := ctx.DeleteAssociation("fleet", "infra"); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, testUser, 0); err != nil {
		t.Fatal(err)
	}
}


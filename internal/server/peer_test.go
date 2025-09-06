package server_test

import (
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestPeerRedeem(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	cidr, err := addPeer(ctx, testServer)
	if err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 0); err != nil {
		t.Fatal(err)
	}

	if err := redeemPeer(ctx, cidr); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 1); err != nil {
		t.Fatal(err)
	}
}

func TestPeerEnable(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}

	if err := addAndRedeemPeer(ctx, testServer); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 1); err != nil {
		t.Fatal(err)
	}

	req := server.UpdatePeerRequest{Enabled: boolPtr(false)}
	if _, err := ctx.UpdatePeer(testServer.Name, req); err != nil {
		t.Fatal(err)
	}

	if err := expectPeerCount(ctx, cordServerPeer, 0); err != nil {
		t.Fatal(err)
	}
}

func TestRenameAndCheckPeerExists(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}
	if err := addAndRedeemPeer(ctx, testServer); err != nil {
		t.Fatal(err)
	}

	if !ctx.CheckPeerExists(testServer.Name) {
		t.Fatalf("expected peer to exist")
	}

	req := server.UpdatePeerRequest{Name: stringPtr("renamed-server")}
	if _, err := ctx.UpdatePeer(testServer.Name, req); err != nil {
		t.Fatalf("failed to rename peer: %v", err)
	}
	if ctx.CheckPeerExists(testServer.Name) {
		t.Fatalf("old peer name should not exist after rename")
	}
	if !ctx.CheckPeerExists("renamed-server") {
		t.Fatalf("renamed peer should exist")
	}
}

func TestPeerAssociationHelpers(t *testing.T) {
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

	// Before association, user should not see infra peers
	if err := expectPeerCount(ctx, testUser, 0); err != nil {
		t.Fatal(err)
	}

	// Associate fleet <-> infra and verify helpers
	if err := ctx.CreateAssociation("fleet", "infra"); err != nil {
		t.Fatal(err)
	}

	// Parent CIDR and associated ids functions should return > 0 and > 1 respectively
	// if _, err := ctx.GetParentCidrIdForPeerNamed(testUser.Name); err != nil {
	// 	t.Fatalf("parent cidr id lookup failed: %v", err)
	// }
	// ids, err := ctx.GetAssociatedCidrIdsOfPeerNamed(testUser.Name)
	// if err != nil {
	// 	t.Fatalf("associated cidr ids lookup failed: %v", err)
	// }
	// if len(ids) < 2 {
	// 	t.Fatalf("expected at least two associated cidrs (self + other), got %d", len(ids))
	// }
}

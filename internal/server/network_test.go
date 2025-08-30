package server_test

import "testing"

func TestCreateNetwork(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}
	if !ctx.CheckPeerExists("cord-server") {
		t.Fatalf("expected cord-server peer to exist after network creation")
	}
}

func TestDeleteNetwork(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}
	if err := ctx.DeleteNetwork(); err != nil {
		t.Fatalf("failed to delete network: %v", err)
	}
}

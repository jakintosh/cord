package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
)

func TestRedeemInvite_Success(t *testing.T) {
	c, teardown := newTestInviteClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/redeem" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				PublicKey:   "server-key",
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: "10.42.0.1/32",
				APIPort:     8443,
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})
	defer teardown()

	result, err := c.RedeemInvitation(t.Context(), "perm-key")
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if result.Network.PublicKey != "server-key" {
		t.Errorf("Network.PublicKey = %q, want server-key", result.Network.PublicKey)
	}
	if result.Network.APIPort != 8443 {
		t.Errorf("Network.APIPort = %d, want 8443", result.Network.APIPort)
	}
	if result.Network.Endpoint != "1.2.3.4:51820" {
		t.Errorf("Network.Endpoint = %q, want 1.2.3.4:51820", result.Network.Endpoint)
	}
	if result.Peer.Route != "10.42.0.5/32" {
		t.Errorf("Peer.Route = %q, want 10.42.0.5/32", result.Peer.Route)
	}
}

func TestRedeemInvite_Forbidden(t *testing.T) {
	c, teardown := newTestInviteClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	_, err := c.RedeemInvitation(t.Context(), "perm-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

// --- test helpers ---

// newTestInviteClient creates an InviteClient pointed at an httptest server.
func newTestInviteClient(
	t *testing.T,
	handler func(w http.ResponseWriter, r *http.Request),
) (
	*client.InviteClient,
	func(),
) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	addr := server.Listener.Addr().String()

	c, err := client.NewInviteClient(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, server.Close
}

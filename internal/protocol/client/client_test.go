package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

func TestListPeers_Success(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/peers" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, []protocol.VisiblePeer{
			{
				Name:      "alice",
				Route:     "10.42.0.5/32",
				PublicKey: "alice-key",
				Endpoints: []protocol.EndpointWitness{
					{
						Endpoint:  "1.2.3.4:51820",
						Timestamp: time.Unix(1718956800, 0),
					},
				},
			},
		})
	})
	defer teardown()

	peers, err := c.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 1 {
		t.Fatalf("got %d peers, want 1", len(peers))
	}
	p := peers[0]
	if p.Name != "alice" {
		t.Errorf("Name = %q, want alice", p.Name)
	}
	if p.Route != "10.42.0.5/32" {
		t.Errorf("Route = %q, want 10.42.0.5/32", p.Route)
	}
	if p.PublicKey != "alice-key" {
		t.Errorf("PublicKey = %q, want alice-key", p.PublicKey)
	}
	if len(p.Endpoints) != 1 {
		t.Fatalf("got %d endpoints, want 1", len(p.Endpoints))
	}
	if p.Endpoints[0].Endpoint != "1.2.3.4:51820" {
		t.Errorf("Endpoint = %q, want 1.2.3.4:51820", p.Endpoints[0].Endpoint)
	}
}

func TestListPeers_Forbidden(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	_, err := c.ListPeers()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

func TestConfirmPeer_Success(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/confirm" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	defer teardown()

	if err := c.ConfirmPeer(); err != nil {
		t.Fatalf("ConfirmPeer: %v", err)
	}
}

func TestConfirmPeer_Forbidden(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	err := c.ConfirmPeer()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

func TestReportEndpoints_Success(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/endpoints" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	defer teardown()

	err := c.ReportEndpoints([]protocol.EndpointSighting{
		{PeerKey: "peer-a", Endpoint: "5.6.7.8:51820"},
	})
	if err != nil {
		t.Fatalf("ReportEndpoints: %v", err)
	}
}

func TestReportEndpoints_Forbidden(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	err := c.ReportEndpoints(nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

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

	result, err := c.RedeemInvitation("perm-key")
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

	_, err := c.RedeemInvitation("perm-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

// --- test helpers ---

// newTestPeerClient creates a PeerClient pointed at an httptest server.
func newTestPeerClient(
	t *testing.T,
	handler func(w http.ResponseWriter, r *http.Request),
) (*PeerClient, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	addr := server.Listener.Addr().String()

	c := NewPeerClient(addr, nil)
	return c, server.Close
}

// newTestInviteClient creates an InviteClient pointed at an httptest server.
func newTestInviteClient(
	t *testing.T,
	handler func(w http.ResponseWriter, r *http.Request),
) (*InviteClient, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	addr := server.Listener.Addr().String()

	c := NewInviteClient(addr, nil)
	return c, server.Close
}

package serverapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

func TestListPeers_Success(t *testing.T) {
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/peers" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, []VisiblePeerDTO{
			{
				Name:      "alice",
				Cidr:      "10.42.0.5/16",
				PublicKey: "alice-key",
				Endpoints: []EndpointWitnessDTO{
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
	if p.Cidr != "10.42.0.5/16" {
		t.Errorf("Cidr = %q, want 10.42.0.5/16", p.Cidr)
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
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/endpoints" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	defer teardown()

	err := c.ReportEndpoints([]EndpointSightingDTO{
		{PeerKey: "peer-a", Endpoint: "5.6.7.8:51820"},
	})
	if err != nil {
		t.Fatalf("ReportEndpoints: %v", err)
	}
}

func TestReportEndpoints_Forbidden(t *testing.T) {
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/redeem" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, RedeemResultDTO{
			NetworkName:  "mynet",
			AssignedCidr: "10.42.0.5/16",
			Server: ServerInfoDTO{
				PublicKey:        "server-key",
				ExternalEndpoint: "1.2.3.4:51820",
				InternalEndpoint: "10.42.0.1:8443",
			},
		})
	})
	defer teardown()

	result, err := c.RedeemInvite(RedeemInviteRequest{
		PermPubKey: "perm-key",
	})
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if result.NetworkName != "mynet" {
		t.Errorf("NetworkName = %q, want mynet", result.NetworkName)
	}
	if result.AssignedCidr != "10.42.0.5/16" {
		t.Errorf("AssignedCidr = %q, want 10.42.0.5/16", result.AssignedCidr)
	}
	if result.Server.PublicKey != "server-key" {
		t.Errorf("Server.PublicKey = %q, want server-key", result.Server.PublicKey)
	}
	if result.Server.InternalEndpoint != "10.42.0.1:8443" {
		t.Errorf("Server.InternalEndpoint = %q, want 10.42.0.1:8443", result.Server.InternalEndpoint)
	}
	if result.Server.ExternalEndpoint != "1.2.3.4:51820" {
		t.Errorf("Server.ExternalEndpoint = %q, want 1.2.3.4:51820", result.Server.ExternalEndpoint)
	}
}

func TestRedeemInvite_Forbidden(t *testing.T) {
	c, teardown := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	_, err := c.RedeemInvite(RedeemInviteRequest{
		PermPubKey: "perm-key",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

// --- test helpers ---

// newTestClient creates a Client pointed at an httptest server whose
// handler is used for both the main and invite API listeners.
func newTestClient(
	t *testing.T,
	handler func(w http.ResponseWriter, r *http.Request),
) (*Client, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	addr := server.Listener.Addr().String()

	c := NewClient(addr, addr, nil)
	return c, server.Close
}

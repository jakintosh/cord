package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
)

func TestGetSnapshot_Success(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/snapshot" {
			wire.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		wire.WriteData(w, http.StatusOK, protocol.VisibleNetworkSnapshot{
			GeneratedAt: time.Unix(1718956800, 0),
			Peers: []protocol.VisiblePeer{{
				Name:      "alice",
				Route:     "10.42.0.5/32",
				PublicKey: "alice-key",
				Endpoints: []protocol.EndpointWitness{
					{
						Endpoint:  "1.2.3.4:51820",
						Timestamp: time.Unix(1718956800, 0),
					},
				},
			}},
			Topology: protocol.TopologyView{
				SubjectPeer: "self",
				Nodes: []protocol.TopologyNode{{
					Name: "self", CIDR: "10.42.0.5/32", Terminal: true,
					PeerName: "self", Subject: true,
				}},
			},
		})
	})
	defer teardown()

	snapshot, err := c.GetSnapshot(t.Context())
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	peers := snapshot.Peers
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

func TestGetSnapshot_Forbidden(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	_, err := c.GetSnapshot(t.Context())
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

	if err := c.ConfirmPeer(t.Context()); err != nil {
		t.Fatalf("ConfirmPeer: %v", err)
	}
}

func TestConfirmPeer_Forbidden(t *testing.T) {
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
	})
	defer teardown()

	err := c.ConfirmPeer(t.Context())
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

	err := c.ReportEndpoints(t.Context(), []protocol.EndpointSighting{
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

	err := c.ReportEndpoints(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !wire.IsStatus(err, http.StatusForbidden) {
		t.Errorf("expected 403, got %v", err)
	}
}

func TestConfirmPeer_CancellationStopsRetry(t *testing.T) {
	called := make(chan struct{}, 1)
	c, teardown := newTestPeerClient(t, func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		wire.WriteError(w, http.StatusServiceUnavailable, "try again")
	})
	defer teardown()

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.ConfirmPeer(ctx)
	}()

	<-called
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ConfirmPeer: got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ConfirmPeer did not stop when its context was cancelled")
	}
}

// --- test helpers ---

// newTestPeerClient creates a PeerClient pointed at an httptest server.
func newTestPeerClient(
	t *testing.T,
	handler func(w http.ResponseWriter, r *http.Request),
) (
	*client.PeerClient,
	func(),
) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handler))
	addr := server.Listener.Addr().String()

	c, err := client.NewPeerClient(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, server.Close
}

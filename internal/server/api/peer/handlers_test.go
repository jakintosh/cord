package peer

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestHandleVisibleSnapshot_Success(t *testing.T) {
	_, api := setupPeerTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/snapshot", nil)
	r.RemoteAddr = "10.0.0.5:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var response struct {
		Data protocol.VisibleNetworkSnapshot `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Topology.SubjectPeer != "alice" {
		t.Fatalf("subject peer = %q, want alice", response.Data.Topology.SubjectPeer)
	}
	if len(response.Data.Topology.Nodes) == 0 {
		t.Fatal("expected projected topology nodes")
	}
}

func TestHandleVisibleSnapshot_IdentityFails(t *testing.T) {
	_, api := setupPeerTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/snapshot", nil)
	r.RemoteAddr = "10.0.0.99:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleReportEndpoints_Success(t *testing.T) {
	env, api := setupPeerTest(t)
	testutil.SeedPeerDB(
		t,
		env.Database,
		"testnet",
		"observed",
		"10.0.0.6/32",
		"peer-key-1",
		false,
		true,
		true,
	)

	body := `[
		{"peer_key": "peer-key-1", "endpoint": "1.2.3.4:51820"}
	]`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/endpoints", strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleReportEndpoints_InvalidJSON(t *testing.T) {
	_, api := setupPeerTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/endpoints", strings.NewReader(`{`))
	r.RemoteAddr = "10.0.0.5:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleReportEndpoints_IdentityFails(t *testing.T) {
	_, api := setupPeerTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/endpoints", strings.NewReader(`[]`))
	r.RemoteAddr = "10.0.0.99:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleConfirmPeer_Success(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Service.CreateRegistration(
		"testnet",
		"alice",
		service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")},
	)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", regs[0].InvitePublicKey, "alice-pub-key"); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}

	api := New(env.Service, "testnet", nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/confirm", nil)
	r.RemoteAddr = "10.0.0.5:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleConfirmPeer_IdentityFails(t *testing.T) {
	_, api := setupPeerTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/confirm", nil)
	r.RemoteAddr = "10.0.0.99:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func setupPeerTest(t *testing.T) (*testutil.ServiceEnv, *API) {
	t.Helper()

	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-pub-key", false, true, true)

	api := New(env.Service, "testnet", nil)

	return env, api
}

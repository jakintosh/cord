package peer

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestHandleVisiblePeers_Success(t *testing.T) {
	_, api := setupPeerTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/peers", nil)
	r.RemoteAddr = "10.0.0.5:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleVisiblePeers_IdentityFails(t *testing.T) {
	_, api := setupPeerTest(t)

	api.resolver = &testutil.FailResolver{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/peers", nil)
	r.RemoteAddr = "10.0.0.99:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleReportEndpoints_Success(t *testing.T) {
	_, api := setupPeerTest(t)

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

	api.resolver = &testutil.FailResolver{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/endpoints", strings.NewReader(`[]`))
	r.RemoteAddr = "10.0.0.99:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleConfirmPeer_Success(t *testing.T) {
	_, api := setupPeerTest(t)

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

	api.resolver = &testutil.FailResolver{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/confirm", nil)
	r.RemoteAddr = "10.0.0.99:12345"

	api.Router().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// --- Test helpers ---

func setupPeerTest(t *testing.T) (*testutil.ServiceEnv, *API) {
	t.Helper()

	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Database.InsertPeer("testnet", &service.Peer{
		Name:      "alice",
		PublicKey: "alice-pub-key",
		Cidr:      "10.0.0.5/32",
		Admin:     false,
		Enabled:   true,
		Confirmed: true,
	}); err != nil {
		t.Fatalf("seed peer: %v", err)
	}

	api := New(env.Service, "testnet", log.Default())

	return env, api
}

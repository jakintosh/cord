package invite_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestHandleRedeemInvite_Success(t *testing.T) {
	_, api := setupInviteTest(t)

	body := `{"perm_pubkey": "new-perm-key"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/redeem", strings.NewReader(body))
	r.RemoteAddr = "10.1.0.2:12345"

	api.Router("testnet").ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleRedeemInvite_InvalidJSON(t *testing.T) {
	_, api := setupInviteTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/redeem", strings.NewReader(`{`))
	r.RemoteAddr = "10.1.0.2:12345"

	api.Router("testnet").ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRedeemInvite_IdentityFails(t *testing.T) {
	_, api := setupInviteTest(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/redeem", strings.NewReader(`{"perm_pubkey": "y"}`))
	r.RemoteAddr = "10.1.0.99:12345"

	api.Router("testnet").ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// --- Test helpers ---

func setupInviteTest(t *testing.T) (*testutil.ServiceEnv, *invite.API) {
	t.Helper()

	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	_, err := env.Database.CreateRegistration("testnet", service.CreateRegistrationParams{
		Name:            "invitee",
		InvitePublicKey: "temp-key-123",
		MainRoute:       "10.0.0.50/32",
		ExpiresAt:       time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC),
		CreatedAt:       time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	}, time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	api := invite.New(env.Service, nil)

	return env, api
}

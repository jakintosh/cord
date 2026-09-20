package identity_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
)

func TestRequire_RejectsMalformedSource(
	t *testing.T,
) {
	for _, remote := range []string{"", "10.0.0.5", "not-an-ip:12345"} {
		t.Run(remote, func(t *testing.T) {
			// Set up the middleware without application dependencies.
			lookupCalls, nextCalls := 0, 0
			handler := identity.Require(func(context.Context, net.IP) (*identity.Peer, error) {
				lookupCalls++
				return &identity.Peer{}, nil
			}, func(http.ResponseWriter, *http.Request) {
				nextCalls++
			}, logging.Discard())

			// Request identity using the malformed source.
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = remote
			w := httptest.NewRecorder()
			handler(w, r)

			// Verify the opaque rejection and absence of downstream work.
			assertIdentityUnknown(t, w)
			if lookupCalls != 0 || nextCalls != 0 {
				t.Fatalf("lookup/next calls = %d/%d, want 0/0", lookupCalls, nextCalls)
			}
		})
	}
}

func TestRequire_RejectsLookupFailure(
	t *testing.T,
) {
	// Set up a lookup failure whose details must remain private.
	lookupCalls, nextCalls := 0, 0
	var source net.IP
	handler := identity.Require(func(_ context.Context, ip net.IP) (*identity.Peer, error) {
		lookupCalls++
		source = ip
		return nil, errors.New("private lookup failure")
	}, func(http.ResponseWriter, *http.Request) {
		nextCalls++
	}, logging.Discard())

	// Request identity with a valid source IP.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.99:12345"
	w := httptest.NewRecorder()
	handler(w, r)

	// Verify the same opaque rejection and no handler invocation.
	assertIdentityUnknown(t, w)
	if lookupCalls != 1 || nextCalls != 0 {
		t.Fatalf("lookup/next calls = %d/%d, want 1/0", lookupCalls, nextCalls)
	}
	if !source.Equal(net.ParseIP("10.0.0.99")) {
		t.Fatalf("lookup IP = %v, want 10.0.0.99", source)
	}
}

func TestRequire_PreservesContextAndIsolatesCallers(
	t *testing.T,
) {
	// Reuse one middleware across IPv4 and IPv6 with distinct identities.
	type requestKey struct{}
	alice := &identity.Peer{PublicKey: "alice-key", Name: "alice"}
	bob := &identity.Peer{PublicKey: "bob-key", Name: "bob"}
	peers := map[string]*identity.Peer{"10.0.0.5": alice, "2001:db8::5": bob}
	var source net.IP
	var lookupValue, handlerValue any
	var caller *identity.Peer
	lookupCalls, nextCalls := 0, 0
	handler := identity.Require(func(ctx context.Context, ip net.IP) (*identity.Peer, error) {
		lookupCalls++
		source = ip
		lookupValue = ctx.Value(requestKey{})
		return peers[ip.String()], nil
	}, func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		caller = identity.Caller(r.Context())
		handlerValue = r.Context().Value(requestKey{})
		w.WriteHeader(http.StatusNoContent)
	}, logging.Discard())

	for _, tc := range []struct {
		remote string
		ip     string
		peer   *identity.Peer
	}{
		{remote: "10.0.0.5:12345", ip: "10.0.0.5", peer: alice},
		{remote: "[2001:db8::5]:23456", ip: "2001:db8::5", peer: bob},
	} {
		// Request identity with a preexisting context value.
		ctx := context.WithValue(context.Background(), requestKey{}, tc.peer.Name)
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		r.RemoteAddr = tc.remote
		w := httptest.NewRecorder()
		handler(w, r)

		// Verify the parsed address, exact peer, and preserved context.
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
		}
		if !source.Equal(net.ParseIP(tc.ip)) {
			t.Fatalf("lookup IP = %v, want %s", source, tc.ip)
		}
		if caller != tc.peer {
			t.Fatalf("caller = %p, want exact peer %p", caller, tc.peer)
		}
		if lookupValue != tc.peer.Name || handlerValue != tc.peer.Name {
			t.Fatalf("lookup/handler context values = %v/%v, want %q", lookupValue, handlerValue, tc.peer.Name)
		}
		if r.Context() != ctx || identity.Caller(r.Context()) != nil {
			t.Fatal("middleware mutated the original request context")
		}
	}
	if lookupCalls != 2 || nextCalls != 2 {
		t.Fatalf("lookup/next calls = %d/%d, want 2/2", lookupCalls, nextCalls)
	}
}

func TestCaller_WithoutIdentityReturnsNil(
	t *testing.T,
) {
	if got := identity.Caller(context.Background()); got != nil {
		t.Fatalf("Caller(background) = %+v, want nil", got)
	}
}

func assertIdentityUnknown(
	t *testing.T,
	w *httptest.ResponseRecorder,
) {
	t.Helper()
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if got, want := w.Body.String(), "{\"error\":{\"message\":\"identity unknown\"}}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

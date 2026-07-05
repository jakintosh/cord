package identity

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequire_Success(t *testing.T) {
	lookup := func(ip net.IP) (*Peer, error) {
		return &Peer{PublicKey: "test-key", Name: "alice"}, nil
	}

	var got *Peer
	handler := Require(lookup, func(w http.ResponseWriter, r *http.Request) {
		got = Caller(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:12345"
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got == nil {
		t.Fatal("Caller returned nil")
	}
	if got.Name != "alice" {
		t.Fatalf("name = %q, want alice", got.Name)
	}
	if got.PublicKey != "test-key" {
		t.Fatalf("public_key = %q, want test-key", got.PublicKey)
	}
}

func TestRequire_BadRemoteAddr(t *testing.T) {
	lookup := func(ip net.IP) (*Peer, error) {
		t.Fatal("lookup should not be called")
		return nil, nil
	}

	handler := Require(lookup, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = ""
	handler(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequire_InvalidRemoteAddr(t *testing.T) {
	lookup := func(ip net.IP) (*Peer, error) {
		t.Fatal("lookup should not be called")
		return nil, nil
	}

	handler := Require(lookup, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "not-an-addr"
	handler(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequire_LookupError(t *testing.T) {
	lookup := func(ip net.IP) (*Peer, error) {
		return nil, errors.New("not found")
	}

	handler := Require(lookup, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.99:12345"
	handler(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequire_BodyWritten(t *testing.T) {
	lookup := func(ip net.IP) (*Peer, error) {
		return nil, errors.New("not found")
	}

	handler := Require(lookup, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next should not be called")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.99:12345"
	handler(w, r)

	if body := w.Body.String(); body != `{"error":{"message":"identity unknown"}}`+"\n" {
		t.Fatalf("body = %q, want %q", body, `{"error":{"message":"identity unknown"}}`)
	}
}

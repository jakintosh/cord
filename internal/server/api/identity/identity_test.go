package identity

import (
	"net"
	"net/http"
	"testing"
)

func TestResolve_ValidRemoteAddr(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:12345"

	peer, err := Resolve(r, &fakeResolver{
		peer: &Peer{PublicKey: "test-key", Name: "alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if peer.Name != "alice" {
		t.Fatalf("name = %q, want alice", peer.Name)
	}
	if peer.PublicKey != "test-key" {
		t.Fatalf("public_key = %q, want test-key", peer.PublicKey)
	}
}

func TestResolve_MissingRemoteAddr(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = ""

	_, err := Resolve(r, &fakeResolver{
		peer: &Peer{PublicKey: "test-key", Name: "alice"},
	})
	if err == nil {
		t.Fatal("expected error for empty RemoteAddr")
	}
}

func TestResolve_InvalidRemoteAddr(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.RemoteAddr = "not-an-addr"

	_, err := Resolve(r, &fakeResolver{
		peer: &Peer{PublicKey: "test-key", Name: "alice"},
	})
	if err == nil {
		t.Fatal("expected error for invalid RemoteAddr")
	}
}

// Test helpers

type fakeResolver struct {
	peer *Peer
}

func (f *fakeResolver) ResolveIdentity(sourceIP net.IP) (*Peer, error) {
	return f.peer, nil
}

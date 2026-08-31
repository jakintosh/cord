package server_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	admin "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

func TestNewRequiresSocketPath(t *testing.T) {
	if _, err := admin.New(""); err == nil {
		t.Fatal("expected empty socket path to fail")
	}
}

func TestClientStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, admin.Status{
			Status:  "ok",
			Health:  "healthy",
			Version: "test",
		})
	})
	client := newTestClient(t, mux)

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Version != "test" {
		t.Fatalf("version = %q, want %q", status.Version, "test")
	}
}

func newTestClient(t *testing.T, handler http.Handler) *admin.Client {
	t.Helper()

	tempRoot := filepath.Join("..", "..", "..", ".opencode-tmp")
	if err := os.MkdirAll(tempRoot, 0755); err != nil {
		t.Fatalf("create test temp root: %v", err)
	}
	dir, err := os.MkdirTemp(tempRoot, "server-admin-")
	if err != nil {
		t.Fatalf("create test temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socketPath := filepath.Join(dir, "admin.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	client, err := admin.New(socketPath)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

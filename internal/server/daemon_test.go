package server_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server"
	adminclient "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

func TestServe_ReportsStatusUntilCanceled(
	t *testing.T,
) {
	dir := daemonDirectory(t)
	socket := filepath.Join(dir, "control.sock")
	admin, err := adminclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var serveErr error
	go func() {
		defer close(done)
		serveErr = server.Serve(ctx, server.Options{
			SocketPath: socket,
			DBPath:     filepath.Join(dir, "server.db"),
			Backend:    "userspace",
			Version:    "daemon-test",
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("daemon did not finish shutdown")
		}
	})

	// Wait for the public control API to become available.
	readyCtx, readyCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer readyCancel()
	for {
		status, err := admin.Status(readyCtx)
		if err == nil {
			if status.Version != "daemon-test" || status.Status != "ok" || status.Health != "healthy" {
				t.Fatalf("unexpected daemon status: %+v", status)
			}
			if len(status.Networks) != 0 {
				t.Fatalf("new daemon should have no networks: %+v", status)
			}
			break
		}
		select {
		case <-done:
			t.Fatalf("daemon exited before cancellation: %v", serveErr)
		case <-readyCtx.Done():
			t.Fatalf("daemon never became available: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}

	select {
	case <-done:
		t.Fatalf("daemon exited before cancellation: %v", serveErr)
	default:
	}

	cancel()
	select {
	case <-done:
		if serveErr != nil {
			t.Fatalf("daemon shutdown: %v", serveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not exit after cancellation")
	}
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("control socket still accepts connections after shutdown")
	}
}

func TestServe_RejectsInvalidStartupOptions(
	t *testing.T,
) {
	for _, tc := range []struct {
		name    string
		socket  string
		backend string
		db      string
		wantErr string
	}{
		{
			name:    "missing socket",
			backend: "userspace",
			db:      "server.db",
			wantErr: "socket path required",
		},
		{
			name:    "unknown backend",
			socket:  "control.sock",
			backend: "unsupported",
			db:      "server.db",
			wantErr: "unknown wireguard backend",
		},
		{
			name:    "database path is a directory",
			socket:  "control.sock",
			backend: "userspace",
			db:      ".",
			wantErr: "open database",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := daemonDirectory(t)
			opts := server.Options{
				Backend: tc.backend,
				DBPath:  filepath.Join(dir, tc.db),
			}
			if tc.socket != "" {
				opts.SocketPath = filepath.Join(dir, tc.socket)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			err := server.Serve(ctx, opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Serve error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func daemonDirectory(
	t *testing.T,
) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".opencode-tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "server-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove daemon directory: %v", err)
		}
	})
	return dir
}

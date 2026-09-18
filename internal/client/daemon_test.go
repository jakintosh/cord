package client

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeRemainsActiveUntilCanceled(t *testing.T) {
	tempRoot, err := filepath.Abs(filepath.Join("..", "..", ".opencode-tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tempRoot, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(tempRoot, "client-daemon-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
		_ = os.Remove(tempRoot)
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var serveErr error
	socket := filepath.Join(dir, "control.sock")
	go func() {
		defer close(done)
		serveErr = Serve(ctx, Options{
			SocketPath: socket,
			DBPath:     filepath.Join(dir, "client.db"),
			Backend:    "userspace",
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

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case <-done:
			t.Fatalf("daemon exited before cancellation: %v", serveErr)
		default:
		}
		response, err := client.Get("http://daemon/status")
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status response = %d", response.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon never became available: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
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
}

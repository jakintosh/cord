package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeHTTP_CancellationStopsRequestsAndServer(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())

	requestStarted := make(chan struct{})
	requestCancelled := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
		close(requestCancelled)
	})

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- ServeHTTP(ctx, listener, handler)
	}()

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
	}()

	waitFor(t, requestStarted, "request to start")
	cancel()

	if err := waitForResult(t, serveDone, "server to stop"); err != nil {
		t.Fatalf("ServeHTTP: %v", err)
	}
	waitFor(t, requestCancelled, "request context to be cancelled")
	waitFor(t, requestDone, "request to finish")
}

func TestServeHTTP_ListenerFailureReturnsError(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- ServeHTTP(ctx, listener, http.NotFoundHandler())
	}()

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	if err := waitForResult(t, serveDone, "listener failure"); err == nil {
		t.Fatal("ServeHTTP returned nil after the listener failed")
	}
}

func TestServeHTTP_ForcesCloseAfterShutdownTimeout(t *testing.T) {
	listener := testListener(t)
	ctx, cancel := context.WithCancel(context.Background())

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseRequest
	})

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveHTTP(ctx, listener, handler, 10*time.Millisecond)
	}()

	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
	}()

	waitFor(t, requestStarted, "request to start")
	cancel()

	err := waitForResult(t, serveDone, "forced shutdown")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ServeHTTP error = %v, want context deadline exceeded", err)
	}
	waitFor(t, requestDone, "connection to close")
	close(releaseRequest)
}

func testListener(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func waitFor(t *testing.T, done <-chan struct{}, event string) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", event)
	}
}

func waitForResult(t *testing.T, done <-chan error, event string) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", event)
		return nil
	}
}

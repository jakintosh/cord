package runtime

import (
	"net"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
)

func TestPlaneServeReportsUnexpectedFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	results := make(chan error, 2)
	plane := &Plane{
		server: &http.Server{Handler: http.NotFoundHandler()},
		log:    logging.Discard(),
		onServeResult: func(err error) {
			results <- err
		},
	}
	plane.serve(listener, listener.Addr().String())

	if err := waitForServeResult(t, results); err != nil {
		t.Fatalf("initial serve result = %v, want nil", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if err := waitForServeResult(t, results); err == nil {
		t.Fatal("unexpected listener exit was not reported")
	}
}

func waitForServeResult(
	t *testing.T,
	results <-chan error,
) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for serve result")
		return nil
	}
}

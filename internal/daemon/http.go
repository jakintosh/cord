package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const shutdownTimeout = 5 * time.Second

// ServeHTTP serves handler on listener until ctx is cancelled or serving
// stops unexpectedly. Cancellation allows in-flight requests a bounded grace
// period before their connections are forcibly closed.
func ServeHTTP(
	ctx context.Context,
	listener net.Listener,
	handler http.Handler,
) error {
	return serveHTTP(ctx, listener, handler, shutdownTimeout)
}

func serveHTTP(
	ctx context.Context,
	listener net.Listener,
	handler http.Handler,
	timeout time.Duration,
) error {
	requestCtx, cancelRequests := context.WithCancel(ctx)
	defer cancelRequests()

	server := &http.Server{
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return requestCtx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	var stopErr error
	select {
	case serveErr := <-errCh:
		cancelRequests()

		// serve errored, so this path always produces an error, even for a nil return
		if serveErr != nil {
			stopErr = fmt.Errorf("serve HTTP: %w", serveErr)
		} else {
			stopErr = errors.New("HTTP server stopped unexpectedly")
		}

	case <-ctx.Done():
		cancelRequests()

		// ctx is already cancelled, so a fresh context for shutdown is needed
		shutdownCtx, cancelShutdown := context.WithTimeout(
			context.Background(),
			timeout,
		)
		defer cancelShutdown()

		// this is graceful shutdown, so we give room for in-flight requests to finish
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			stopErr = fmt.Errorf("shut down HTTP server: %w", shutdownErr)
		} else {
			// fully graceful shutdown case
			return nil
		}
	}

	// force-close server and fold any close error into the result
	closeErr := server.Close()
	return errors.Join(stopErr, closeErr)
}

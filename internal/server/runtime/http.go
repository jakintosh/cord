package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
)

// httpServer owns configured HTTP serving across starts, failures, and stops.
type httpServer struct {
	address string
	handler http.Handler
	log     *slog.Logger
	timeout time.Duration

	run     *httpRun
	lastErr error
}

// httpRun holds one serving attempt. Its result cannot affect a later attempt.
type httpRun struct {
	server *http.Server
	cancel context.CancelFunc
	done   chan struct{}
	err    error // Written before done closes; read only after it closes.
}

// status reads the serving result without repairing the server.
func (s *httpServer) status() resource.Status {
	if s.run != nil {
		select {
		case <-s.run.done:
			return resource.Status{
				State: resource.Failed,
				Error: s.run.err,
			}
		default:
			return resource.Status{
				State: resource.Running,
			}
		}
	}

	if s.lastErr != nil {
		return resource.Status{
			State: resource.Failed,
			Error: s.lastErr,
		}
	}

	return resource.Status{
		State: resource.Stopped,
	}
}

// reconcile leaves healthy serving alone and cleans up before restarting.
func (s *httpServer) reconcile(
	ctx context.Context,
) error {
	if s.status().State == resource.Running {
		return nil
	}

	cleanup, cancel := context.WithTimeout(context.Background(), s.timeout)
	err := s.close(cleanup)
	cancel()
	if err != nil {
		return err
	}

	return s.start(ctx)
}

// start opens one HTTP serving attempt, owned by the runtime loop.
func (s *httpServer) start(
	ctx context.Context,
) error {
	if s.run != nil {
		return fmt.Errorf("http: server already started")
	}

	if err := ctx.Err(); err != nil {
		s.lastErr = err
		return s.lastErr
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		s.lastErr = fmt.Errorf("http listen on %s: %w", s.address, err)
		return s.lastErr
	}

	requestCtx, cancel := context.WithCancel(ctx)
	run := &httpRun{
		server: &http.Server{
			Addr:    listener.Addr().String(),
			Handler: s.handler,
			BaseContext: func(net.Listener) context.Context {
				return requestCtx
			},
		},
		cancel: cancel,
		done:   make(chan struct{}),
	}
	s.run = run
	s.lastErr = nil

	go func() {
		run.err = run.server.Serve(listener)
		if !errors.Is(run.err, http.ErrServerClosed) {
			s.log.Error(
				"http server exited",
				"addr", run.server.Addr,
				"err", run.err,
			)
		}
		close(run.done)
	}()

	return nil
}

// close cancels request contexts before draining. Handlers waiting for runtime
// must be able to leave without requiring another turn of the command loop.
func (s *httpServer) close(
	ctx context.Context,
) error {
	if s.run == nil {
		return nil
	}

	run := s.run

	select {

	case <-run.done:
		s.lastErr = run.err

	default:
		// still running, no run.err
		break
	}

	run.cancel()
	err := run.server.Shutdown(ctx)
	if err != nil {
		err = fmt.Errorf("http shutdown: %w", err)
		if closeErr := run.server.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("http force close: %w", closeErr))
		}
	}

	<-run.done
	s.run = nil
	s.lastErr = errors.Join(s.lastErr, err)

	return err
}

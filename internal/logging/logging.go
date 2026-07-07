// Package logging constructs the loggers used by the cord daemons.
// There are exactly two modes: the default (Info and up, what belongs
// in a journal) and debug (everything, for tracing handshakes and
// reconciliation). Everything downstream sees a plain *slog.Logger;
// scope is carried by child loggers (log.With("network", name)) built
// at construction time, never repeated at call sites.
package logging

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

// New returns the root logger for a daemon process, writing text to
// stderr. Debug enables verbose tracing; otherwise Info and up.
func New(
	debug bool,
) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
}

// Discard returns a logger that drops everything. Used as the nil
// fallback in constructors and as the default in tests.
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// Middleware logs one Debug line per HTTP request: method, path,
// remote address, response status, and duration.
func Middleware(
	log *slog.Logger,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(sw, r)
		log.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"status", sw.status,
			"duration", time.Since(start).Round(time.Microsecond),
		)
	})
}

// statusWriter captures the response status code for request logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(
	status int,
) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

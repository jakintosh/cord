package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	inviteapi "git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	peerapi "git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = "/tmp/cord-server.sock"

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/server.db"

// wakeBuffer is the depth of the channel the service uses to tell the
// runtime that a network changed. Sends are dropped when it is full;
// the runtime's periodic pass catches up.
const wakeBuffer = 16

// Options configures the server composition root.
type Options struct {
	// SocketPath is the Unix socket path for the daemon control API.
	SocketPath string

	// DBPath is the filesystem path to the SQLite database file.
	DBPath string

	// Backend selects the WireGuard implementation: "auto" (default),
	// "kernel", or "userspace".
	Backend string

	// Version is the daemon build version, surfaced over the status API.
	Version string

	// Debug enables verbose logging: requests, WireGuard handshakes,
	// and reconciliation detail.
	Debug bool
}

// Serve is the production composition root for the cord server daemon.
// It opens dependencies, wires them database → service → APIs → runtime,
// starts the daemon on a Unix socket, and blocks until the context is
// cancelled.
func Serve(
	ctx context.Context,
	opts Options,
) error {
	if opts.DBPath == "" {
		opts.DBPath = DefaultDBPath
	}
	if opts.SocketPath == "" {
		return fmt.Errorf("server: socket path required")
	}

	log := logging.New(opts.Debug)

	backend, err := wireguard.ParseBackendType(opts.Backend)
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}

	wg, err := wireguard.NewManager(wireguard.Options{
		Backend: backend,
		Logger:  log,
	})
	if err != nil {
		return fmt.Errorf("server: new wireguard: %w", err)
	}

	db, err := database.Open(database.Options{
		Path: opts.DBPath,
		WAL:  true,
	})
	if err != nil {
		return fmt.Errorf("server: open database: %w", err)
	}
	defer db.Close()

	wake := make(chan string, wakeBuffer)

	svc, err := service.New(service.Options{
		Store:  db,
		Clock:  time.Now,
		Logger: log,
		Wake:   wake,
	})
	if err != nil {
		return fmt.Errorf("server: new service: %w", err)
	}

	peer := peerapi.New(svc, log.With("api", "peer"))
	invite := inviteapi.New(svc, log.With("api", "invite"))

	rt, err := runtime.New(runtime.Options{
		Service:   svc,
		WireGuard: wg,
		Peer:      peer,
		Invite:    invite,
		Wake:      wake,
		Clock:     time.Now,
		Logger:    log,
	})
	if err != nil {
		return fmt.Errorf("server: new runtime: %w", err)
	}

	api, err := admin.New(admin.Options{
		Service: svc,
		Runtime: rt,
		Logger:  log.With("api", "admin"),
		Version: opts.Version,
	})
	if err != nil {
		return fmt.Errorf("server: new api: %w", err)
	}

	if err := rt.Start(ctx); err != nil {
		return fmt.Errorf("server: start runtime: %w", err)
	}
	defer rt.Stop()

	ln, err := listenUnix(opts.SocketPath)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Info(
		"daemon listening",
		"socket",
		opts.SocketPath,
		"version",
		opts.Version,
	)

	return serveHTTP(ctx, ln, api.Router())
}

// listenUnix removes any stale socket at path, creates a new Unix
// listener, sets permissive permissions, and returns it.
func listenUnix(
	path string,
) (
	net.Listener,
	error,
) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("server: remove socket: %w", err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("server: listen: %w", err)
	}

	if err := os.Chmod(path, 0666); err != nil {
		ln.Close()
		return nil, fmt.Errorf("server: chmod socket: %w", err)
	}

	return ln, nil
}

// serveHTTP starts an HTTP server on ln, blocks until ctx is cancelled,
// then gracefully shuts down with a 5-second timeout. Requests inherit
// ctx, so in-flight work sees the shutdown.
func serveHTTP(
	ctx context.Context,
	ln net.Listener,
	handler http.Handler,
) error {
	srv := &http.Server{
		Handler:     handler,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

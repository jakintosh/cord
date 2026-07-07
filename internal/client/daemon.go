package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Options configures the client daemon composition root. Both fields are
// required for full operation.
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
	// and sync/scan/report detail.
	Debug bool
}

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = "/tmp/cord-client.sock"

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/client.db"

// Serve is the production composition root for the cord client daemon.
// It opens dependencies, constructs the service and API, starts the
// daemon on a Unix socket, and blocks until the context is cancelled.
func Serve(
	ctx context.Context,
	opts Options,
) error {
	if opts.DBPath == "" {
		opts.DBPath = DefaultDBPath
	}
	if opts.SocketPath == "" {
		return fmt.Errorf("client: socket path required")
	}

	log := logging.New(opts.Debug)

	deps, err := initDependencies(opts, log)
	if err != nil {
		return err
	}
	defer deps.close()

	ln, err := listenUnix(opts.SocketPath)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Info("daemon listening", "socket", opts.SocketPath, "version", opts.Version)
	return serveHTTP(ctx, ln, deps.api.Router())
}

// daemonDeps holds the constructed dependencies for the daemon's lifetime.
type daemonDeps struct {
	db  *database.DB
	svc *service.Service
	api *api.API
}

func (d *daemonDeps) close() {
	d.svc.Close()
	d.db.Close()
}

// initDependencies constructs and wires the database, WireGuard manager,
// service, and API server. The caller must call close() on the returned
// deps to shut down cleanly.
func initDependencies(
	opts Options,
	log *slog.Logger,
) (
	*daemonDeps,
	error,
) {
	dbOpts := database.Options{
		Path: opts.DBPath,
		WAL:  true,
	}
	db, err := database.Open(dbOpts)
	if err != nil {
		return nil, fmt.Errorf("client: open database: %w", err)
	}

	backend, err := wireguard.ParseBackendType(opts.Backend)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("client: %w", err)
	}
	wgOpts := wireguard.Options{
		Backend: backend,
		Logger:  log,
	}
	wg, err := wireguard.NewManager(wgOpts)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("client: new wireguard: %w", err)
	}

	svcOpts := service.Options{
		Store:     db,
		WireGuard: wg,
		Clock:     time.Now,
		Logger:    log,
	}
	svc, err := service.New(svcOpts)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("client: new service: %w", err)
	}

	if err := svc.Start(); err != nil {
		db.Close()
		return nil, fmt.Errorf("client: start service: %w", err)
	}

	apiOpts := api.Options{
		Service: svc,
		Logger:  log.With("api", "control"),
		Version: opts.Version,
	}
	apiServer, err := api.New(apiOpts)
	if err != nil {
		svc.Close()
		db.Close()
		return nil, fmt.Errorf("client: new api: %w", err)
	}

	return &daemonDeps{
		db:  db,
		svc: svc,
		api: apiServer,
	}, nil
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
		return nil, fmt.Errorf("client: remove socket: %w", err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("client: listen: %w", err)
	}

	if err := os.Chmod(path, 0666); err != nil {
		ln.Close()
		return nil, fmt.Errorf("client: chmod socket: %w", err)
	}

	return ln, nil
}

// serveHTTP starts an HTTP server on ln, blocks until ctx is cancelled,
// then gracefully shuts down with a 5-second timeout.
func serveHTTP(
	ctx context.Context,
	ln net.Listener,
	handler http.Handler,
) error {
	srv := &http.Server{
		Handler: handler,
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

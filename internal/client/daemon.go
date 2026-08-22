package client

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = "/var/run/cord/client.sock"

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/client.db"

// wakeBuffer is the depth of the channel the service uses to tell the
// runtime that a network changed. Sends are dropped when it is full;
// the runtime's periodic pass catches up.
const wakeBuffer = 16

// Options configures the client daemon composition root.
type Options struct {
	// SocketPath is the Unix socket path for the daemon control API.
	SocketPath string

	// SocketMode controls which local users may connect to the control API.
	// Zero uses daemon.DefaultSocketMode.
	SocketMode fs.FileMode

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

// Serve is the production composition root for the cord client daemon.
// It opens dependencies, wires them database → service → runtime → API,
// starts the daemon on a Unix socket, and blocks until cancellation or
// control-endpoint failure.
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

	ln, err := daemon.ListenUnix(opts.SocketPath, opts.SocketMode)
	if err != nil {
		return fmt.Errorf("client: control socket: %w", err)
	}
	defer ln.Close()

	backend, err := wireguard.ParseBackendType(opts.Backend)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}

	wg, err := wireguard.NewManager(wireguard.Options{
		Backend: backend,
		Logger:  log,
	})
	if err != nil {
		return fmt.Errorf("client: new wireguard: %w", err)
	}

	db, err := database.Open(database.Options{
		Path: opts.DBPath,
		WAL:  true,
	})
	if err != nil {
		return fmt.Errorf("client: open database: %w", err)
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
		return fmt.Errorf("client: new service: %w", err)
	}

	rt, err := runtime.New(runtime.Options{
		Service:   svc,
		WireGuard: wg,
		Wake:      wake,
		Clock:     time.Now,
		Logger:    log,
	})
	if err != nil {
		return fmt.Errorf("client: new runtime: %w", err)
	}

	apiServer, err := api.New(api.Options{
		Service: svc,
		Runtime: rt,
		Logger:  log.With("api", "control"),
		Version: opts.Version,
	})
	if err != nil {
		return fmt.Errorf("client: new api: %w", err)
	}

	if err := rt.Start(ctx); err != nil {
		return fmt.Errorf("client: start runtime: %w", err)
	}
	defer rt.Stop()

	log.Info(
		"daemon listening",
		"socket",
		opts.SocketPath,
		"version",
		opts.Version,
	)

	return daemon.ServeHTTP(ctx, ln, apiServer.Router())
}

package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
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

	// SyncInterval controls the client sync interval. Defaults to 30s
	// when zero.
	SyncInterval time.Duration
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
	dbOpts := database.Options{
		Path: opts.DBPath,
		WAL:  true,
	}
	db, err := database.Open(dbOpts)
	if err != nil {
		return fmt.Errorf("client: open database: %w", err)
	}
	defer db.Close()

	backend, err := wireguard.ParseBackendType(opts.Backend)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	wgOpts := wireguard.Options{
		Backend: backend,
	}
	wg, err := wireguard.NewManager(wgOpts)
	if err != nil {
		return fmt.Errorf("client: new wireguard: %w", err)
	}

	svcOpts := service.Options{
		Store:        db,
		WireGuard:    wg,
		Clock:        time.Now,
		Logger:       log.Default(),
		SyncInterval: opts.SyncInterval,
	}
	svc, err := service.New(svcOpts)
	if err != nil {
		return fmt.Errorf("client: new service: %w", err)
	}

	if err := svc.Start(ctx); err != nil {
		return fmt.Errorf("client: start service: %w", err)
	}
	defer svc.Close()

	apiOpts := api.Options{
		Service: svc,
	}
	apiServer, err := api.New(apiOpts)
	if err != nil {
		return fmt.Errorf("client: new api: %w", err)
	}

	d, err := daemon.New(opts.SocketPath, apiServer.Router())
	if err != nil {
		return fmt.Errorf("client: new daemon: %w", err)
	}

	return d.Run(ctx)
}

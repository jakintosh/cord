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
)

// Options configures the client daemon composition root. Both fields are
// required for full operation.
type Options struct {
	// SocketPath is the Unix socket path for the daemon control API.
	SocketPath string

	// DBPath is the filesystem path to the SQLite database file.
	DBPath string
}

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = "/tmp/cord-client.sock"

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/client.db"

// Run is the production composition root for the cord client daemon.
// It opens dependencies, constructs the service and API, starts the
// daemon on a Unix socket, and blocks until the context is cancelled.
func Run(
	ctx context.Context,
	opts Options,
) error {
	if opts.SocketPath == "" {
		return fmt.Errorf("client: socket path required")
	}
	if opts.DBPath == "" {
		opts.DBPath = DefaultDBPath
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

	svcOpts := service.Options{
		Store:  db,
		WG:     nil,
		Clock:  time.Now,
		Logger: log.Default(),
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

package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// Options configures the server composition root. Both fields are
// required for full operation.
type Options struct {
	// SocketPath is the Unix socket path for the daemon control API.
	SocketPath string

	// DBPath is the filesystem path to the SQLite database file.
	DBPath string
}

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/server.db"

// Serve is the production composition root for the cord server daemon.
// It opens dependencies, constructs the service and API, starts the
// daemon on a Unix socket, and blocks until the context is cancelled.
func Serve(
	ctx context.Context,
	opts Options,
) error {
	if opts.SocketPath == "" {
		return fmt.Errorf("server: socket path required")
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
		return fmt.Errorf("server: open database: %w", err)
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
		return fmt.Errorf("server: new service: %w", err)
	}

	apiOpts := api.Options{
		Service: svc,
	}
	apiServer, err := api.New(apiOpts)
	if err != nil {
		return fmt.Errorf("server: new api: %w", err)
	}

	d, err := daemon.New(opts.SocketPath, apiServer.Router())
	if err != nil {
		return fmt.Errorf("server: new daemon: %w", err)
	}

	return d.Run(ctx)
}

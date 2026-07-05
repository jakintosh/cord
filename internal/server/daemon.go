package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	inviteApi "git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	peerApi "git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = "/tmp/cord-server.sock"

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/server.db"

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
}

// Serve is the production composition root for the cord server daemon.
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
		return fmt.Errorf("server: socket path required")
	}
	backend, err := wireguard.ParseBackendType(opts.Backend)
	if err != nil {
		return fmt.Errorf("server: %w", err)
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

	wgOpts := wireguard.Options{
		Backend: backend,
	}
	wg, err := wireguard.NewManager(wgOpts)
	if err != nil {
		return fmt.Errorf("server: new wireguard: %w", err)
	}

	var svc *service.Service
	svcOpts := service.Options{
		Store:     db,
		WireGuard: wg,
		Clock:     time.Now,
		Logger:    log.Default(),
		// TODO: I don't like the way the APIFactory has this circular dependency
		APIFactory: func(network string) service.APIHandlers {
			return service.APIHandlers{
				Main:   peerApi.New(svc, network, log.Default()).Router(),
				Invite: inviteApi.New(svc, network, log.Default()).Router(),
			}
		},
	}
	svc, err = service.New(svcOpts)
	if err != nil {
		return fmt.Errorf("server: new service: %w", err)
	}

	if err := svc.Start(); err != nil {
		return fmt.Errorf("server: start networks: %w", err)
	}

	apiOpts := admin.Options{
		Service: svc,
		Version: opts.Version,
	}
	apiServer, err := admin.New(apiOpts)
	if err != nil {
		return fmt.Errorf("server: new api: %w", err)
	}

	d, err := daemon.New(opts.SocketPath, apiServer.Router())
	if err != nil {
		return fmt.Errorf("server: new daemon: %w", err)
	}

	return d.Run(ctx)
}

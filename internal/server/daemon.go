package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = "/tmp/cord-server.sock"

// DefaultDBPath is the default database path used when none is provided.
const DefaultDBPath = "data/server.db"

// Options configures the server composition root. Both fields are
// required for full operation.
type Options struct {
	// SocketPath is the Unix socket path for the daemon control API.
	SocketPath string

	// DBPath is the filesystem path to the SQLite database file.
	DBPath string

	// Backend selects the WireGuard implementation: "auto" (default),
	// "kernel", or "userspace".
	Backend string

	// ReconcileInterval controls the server reconciliation interval.
	// Defaults to 10s when zero.
	ReconcileInterval time.Duration
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
	wg, err := wireguard.New(wgOpts)
	if err != nil {
		return fmt.Errorf("server: new wireguard: %w", err)
	}

	var svc *service.Service
	svcOpts := service.Options{
		Store:             db,
		WG:                wg,
		Clock:             time.Now,
		Logger:            log.Default(),
		ReconcileInterval: opts.ReconcileInterval,
		APIFactory: func(network string) service.APIHandlers {
			peerAPI := peer.New(svc, network, log.Default())
			inviteAPI := invite.New(svc, network, log.Default())

			return service.APIHandlers{
				Main:   peerAPI.Router(),
				Invite: inviteAPI.Router(),
			}
		},
	}
	svc, err = service.New(svcOpts)
	if err != nil {
		return fmt.Errorf("server: new service: %w", err)
	}

	if err := startEnabledNetworks(ctx, svc); err != nil {
		return fmt.Errorf("server: start networks: %w", err)
	}

	apiOpts := admin.Options{
		Service: svc,
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

// startEnabledNetworks iterates all persisted networks and starts
// those marked as enabled. Non-fatal: a single network failure is
// logged but does not prevent others from starting.
func startEnabledNetworks(
	ctx context.Context,
	svc *service.Service,
) error {
	names, err := svc.ListNetworks()
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	var lastErr error
	for _, name := range names {
		nw, err := svc.GetNetwork(name)
		if err != nil {
			log.Printf("start networks: get %q: %v", name, err)
			lastErr = err
			continue
		}
		if !nw.Enabled {
			continue
		}
		if err := svc.StartNetwork(ctx, name); err != nil {
			log.Printf("start networks: start %q: %v", name, err)
			lastErr = err
		}
	}
	return lastErr
}

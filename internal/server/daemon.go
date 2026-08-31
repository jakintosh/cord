package server

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	adminapi "git.studiopollinator.com/pollinator/cord/internal/server/api/admin"
	inviteapi "git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	peerapi "git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	adminclient "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

// DefaultSocketPath is the default Unix socket path used when none is
// provided.
const DefaultSocketPath = adminclient.DefaultSocketPath

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
	// and reconciliation detail.
	Debug bool
}

// Serve is the production composition root for the cord server daemon.
// It opens dependencies, wires them database → service → APIs → runtime,
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
		return fmt.Errorf("server: socket path required")
	}

	log := logging.New(opts.Debug)

	ln, err := daemon.ListenUnix(opts.SocketPath, opts.SocketMode)
	if err != nil {
		return fmt.Errorf("server: control socket: %w", err)
	}
	defer ln.Close()

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

	api, err := adminapi.New(adminapi.Options{
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

	log.Info(
		"daemon listening",
		"socket",
		opts.SocketPath,
		"version",
		opts.Version,
	)

	return daemon.ServeHTTP(ctx, ln, api.Router())
}

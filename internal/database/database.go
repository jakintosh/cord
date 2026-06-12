package database

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"syscall"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Scanner interface {
	Scan(dest ...any) error
}

// Options configures a database opened by OpenServer or OpenClient.
type Options struct {
	Name      string // network name; the database file is <Name>.db inside Dir
	Dir       string // data directory, or ":memory:" for an in-memory database
	WAL       bool   // enable write-ahead logging (file-backed databases)
	MustExist bool   // require the database file to already exist (file-backed databases)
}

// ServerDB is the SQLite adapter backing a coordination server's
// network state.
type ServerDB struct {
	Conn *sql.DB
	dir  string
}

var _ server.ServerStore = (*ServerDB)(nil)

// OpenServer opens (creating and migrating as needed) the server
// database for a network and returns a handle ready for use.
func OpenServer(
	opts Options,
) (
	*ServerDB,
	error,
) {
	conn, err := openConn(opts)
	if err != nil {
		return nil, err
	}

	if err := migrate(conn, serverMigrations); err != nil {
		conn.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &ServerDB{Conn: conn, dir: opts.Dir}, nil
}

func (s *ServerDB) Close() error {
	return s.Conn.Close()
}

// openConn opens a SQLite connection with the settings every cord
// database requires. Writes are serialized through a single connection;
// with modernc sqlite this also keeps ':memory:' databases coherent
// (each pooled connection would otherwise see its own empty database).
func openConn(
	opts Options,
) (
	*sql.DB,
	error,
) {
	target := ":memory:"
	if opts.Dir != ":memory:" {
		target = dbPath(opts.Name, opts.Dir)
		if opts.MustExist {
			if _, err := os.Stat(target); err != nil {
				return nil, fmt.Errorf(
					"%w: no database for network '%s'",
					server.ErrNotFound, opts.Name,
				)
			}
		} else if err := os.MkdirAll(opts.Dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory '%s': %w", opts.Dir, err)
		}
	}

	conn, err := sql.Open("sqlite", target)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1)

	if _, err := conn.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := conn.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	if opts.WAL {
		if _, err := conn.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			conn.Close()
			return nil, fmt.Errorf("enable wal mode: %w", err)
		}
	}

	return conn, nil
}

func (s *ServerDB) Create(
	name string,
	root *net.IPNet,
	serverPubKey string,
) error {
	// Begin transaction
	tx, err := s.Conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert root CIDR with explicit id=1
	prefix, length := root.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(root)
	if _, err := tx.Exec(`
        INSERT INTO cidr (id, name, cidr, length, prefix, base, last)
        VALUES (1, ?1, ?2, ?3, ?4, ?5, ?6);`,
		name,
		root.String(),
		length,
		prefix,
		base,
		last,
	); err != nil {
		return CheckSqliteErr("adding root cidr", err)
	}

	// Derive server IP and prefix from root
	serverIP := utils.GetFirstAssignableIpFromCidr(root)
	ipPrefix := len(serverIP) * 8

	// Insert server peer directly
	if _, err := tx.Exec(`
        INSERT INTO peer (name, ip, prefix, public_key, admin, enabled, confirmed)
        VALUES ('cord-server', ?1, ?2, ?3, 1, 1, 1);`,
		serverIP,
		ipPrefix,
		serverPubKey,
	); err != nil {
		return CheckSqliteErr("adding server peer", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit create tx: %w", err)
	}

	return nil
}

func (s *ServerDB) Delete(
	name string,
) error {

	if s.dir == ":memory:" {
		return nil
	}

	if err := s.Conn.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}
	if err := removeDbFiles(dbPath(name, s.dir), false); err != nil {
		return err
	}
	if err := os.Remove(s.dir); err != nil &&
		!os.IsNotExist(err) &&
		!errors.Is(err, syscall.ENOTEMPTY) &&
		!errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("failed to delete empty data directory: %w", err)
	}
	return nil
}

func dbPath(
	name string,
	dataPath string,
) string {
	dbName := name + ".db"
	return path.Join(dataPath, dbName)
}

// removeDbFiles removes a database file and its WAL sidecars. The
// sidecars are always allowed to be missing; missingOk extends that to
// the database file itself.
func removeDbFiles(
	dbPath string,
	missingOk bool,
) error {
	if err := os.Remove(dbPath); err != nil {
		if !missingOk || !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete database: %w", err)
		}
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete database sidecar: %w", err)
		}
	}
	return nil
}

func ResultsEmpty(
	result sql.Result,
) bool {
	count, err := result.RowsAffected()
	if err != nil {
		return false
	}
	return count == 0
}

// CheckSqliteErr translates database errors into service-level
// sentinels (server.ErrNotFound, server.ErrConflict) where the cause
// is recognizable, so callers never inspect SQL details.
func CheckSqliteErr(
	context string,
	err error,
) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no rows while %s", server.ErrNotFound, context)
	}

	if sqliteErr, ok := err.(*sqlite.Error); ok {
		if sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("%w: unique constraint violation while %s", server.ErrConflict, context)
		}
		return fmt.Errorf(
			"SQLite error (%d) while %s: %s",
			sqliteErr.Code(), context, sqliteErr.Error(),
		)
	}

	return fmt.Errorf("database error while %s: %w", context, err)
}

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Scanner interface {
	Scan(dest ...any) error
}

type SQLiteStore struct {
	path    string
	walMode bool
	db      *sql.DB
}

func Init(
	name string,
	path string,
	walMode bool,
) (
	*SQLiteStore,
	error,
) {

	// create the store
	store := &SQLiteStore{
		path:    path,
		walMode: walMode,
		db:      nil,
	}

	// open database connection
	if err := store.Open(name); err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	// optional WAL config
	if store.walMode {

		// enable write ahead logging mode
		_, err := store.db.Exec("PRAGMA journal_mode = WAL;")
		if err != nil {
			log.Fatalf("could not enable WAL mode: %v", err)
		}

		// disallow multiple connections for serial writes
		store.db.SetMaxOpenConns(1)

		// increase timeout so waiting writes can finish
		_, err = store.db.Exec("PRAGMA busy_timeout = 5000;")
		if err != nil {
			log.Fatalf("could not set busy timeout: %v", err)
		}
	}

	if _, err := store.db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		log.Fatalf("couldn't enable foreign keys: %v\n", err)
	}

	// run all migrations
	if err := migrate(store.db); err != nil {
		log.Fatalf("could not migrate database: %v", err)
	}

	return store, nil
}

func (s *SQLiteStore) Open(
	name string,
) error {

	var dbPath string
	if s.path == ":memory:" {
		dbPath = ":memory:"
	} else {
		os.MkdirAll(s.path, os.ModePerm)
		dbPath = GetPath(name, s.path)
	}

	var err error
	s.db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("sqlite error: %w\n", err)
	}
	return nil
}

func (s *SQLiteStore) Create(
	name string,
	root *net.IPNet,
	serverPubKey string,
) error {
	// Begin transaction
	tx, err := s.db.Begin()
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

func (s *SQLiteStore) Delete(
	name string,
) error {

	if s.path == ":memory:" {
		return nil
	}

	dbPath := GetPath(name, s.path)
	if err := os.Remove(dbPath); err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

func Open(
	name string,
	path string,
) (
	*sql.DB,
	error,
) {

	var dbPath string
	if path == ":memory:" {
		dbPath = ":memory:"
	} else {
		os.MkdirAll(path, os.ModePerm)
		dbPath = GetPath(name, path)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite error: %w\n", err)
	}
	return db, nil
}
func InitTable(
	db *sql.DB,
	name string,
	sql string,
) error {

	if _, err := db.Exec(sql); err != nil {
		return fmt.Errorf("failed to init '%s' table schema: %w\n", name, err)
	}
	return nil
}

func EnableForeignKeys(
	db *sql.DB,
) error {
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("couldn't enable foreign keys: %w\n", err)
	}
	return nil
}

func GetPath(
	name string,
	dataPath string,
) string {
	dbName := name + ".db"
	return path.Join(dataPath, dbName)
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

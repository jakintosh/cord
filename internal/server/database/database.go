package database

import (
	"database/sql"
	"errors"
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Scanner interface {
	Scan(dest ...any) error
}

var errScan = errors.New("scan failed")

type Options struct {
	Path string
	WAL  bool
}

type DB struct {
	Conn *sql.DB
}

var _ service.Store = (*DB)(nil)

func Open(
	opts Options,
) (
	*DB,
	error,
) {
	conn, err := sql.Open("sqlite", opts.Path)
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

	db := &DB{
		Conn: conn,
	}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.Conn.Close()
}

func CheckSqliteErr(
	context string,
	err error,
) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no rows while %s", service.ErrNotFound, context)
	}

	if sqliteErr, ok := err.(*sqlite.Error); ok {
		if sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("%w: unique constraint violation while %s", service.ErrConflict, context)
		}
		return fmt.Errorf(
			"SQLite error (%d) while %s: %s",
			sqliteErr.Code(), context, sqliteErr.Error(),
		)
	}

	return fmt.Errorf("database error while %s: %w", context, err)
}

func boolToInt(
	b bool,
) int64 {
	if b {
		return 1
	}
	return 0
}

func validOptBool(
	b *bool,
) any {
	if b == nil {
		return nil
	}
	if *b {
		return int64(1)
	}
	return int64(0)
}

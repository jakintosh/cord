package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"

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
	var err error
	store.db, err = Open(name, store.path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	// optional WAL config
	if store.walMode {

		// enable write ahead logging mode
		_, err = store.db.Exec("PRAGMA journal_mode = WAL;")
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

func Open(
	name string,
	dataPath string,
) (
	*sql.DB,
	error,
) {

	var dbPath string
	if dataPath == ":memory:" {
		dbPath = ":memory:"
	} else {
		os.MkdirAll(dataPath, os.ModePerm)
		dbPath = GetPath(name, dataPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite error: %w\n", err)
	}
	return db, nil
}

func Delete(
	name string,
	dataPath string,
) error {

	dbPath := GetPath(name, dataPath)
	if err := os.Remove(dbPath); err != nil {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
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

func CheckSqliteErr(
	context string,
	err error,
) error {

	if err == nil {
		return nil
	}

	if sqliteErr, ok := err.(*sqlite.Error); ok {
		if sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("Unique constraint violation while %s", context)
		} else {
			return fmt.Errorf(
				"SQLite error (%d) while %s: %s",
				sqliteErr.Code(), context, sqliteErr.Error(),
			)
		}
	} else {
		return fmt.Errorf("other database error while %s: %w", context, err)
	}
}

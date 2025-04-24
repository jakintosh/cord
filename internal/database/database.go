package database

import (
	"database/sql"
	"fmt"
	"os"
	"path"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

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
		return nil, fmt.Errorf("failed to open to database: %w\n", err)
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

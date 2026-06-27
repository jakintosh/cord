package database_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
)

func TestOpen(t *testing.T) {
	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestOpen_UserVersionSet(t *testing.T) {
	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.Conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
	}
}

func TestOpen_TablesExist(t *testing.T) {
	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"network", "peer"} {
		var name string
		err := db.Conn.QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?1`,
			table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q should exist: %v", table, err)
		}
	}
}

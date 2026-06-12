package database_test

import (
	"database/sql"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/database"
)

func schemaVersion(t *testing.T, conn *sql.DB) int {
	t.Helper()
	var version int
	if err := conn.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatalf("failed to read user_version: %v", err)
	}
	return version
}

func assertTableExists(t *testing.T, conn *sql.DB, table string) {
	t.Helper()
	var name string
	err := conn.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = ?1;`,
		table,
	).Scan(&name)
	if err != nil {
		t.Fatalf("expected table '%s' to exist: %v", table, err)
	}
}

func TestOpenServer_MigratesToVersion1(t *testing.T) {
	store := setupTestDB(t)

	if version := schemaVersion(t, store.Conn); version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
	}
}

func TestOpenServer_CreatesAllTables(t *testing.T) {
	store := setupTestDB(t)

	for _, table := range []string{"association", "cidr", "endpoint", "invite", "peer"} {
		assertTableExists(t, store.Conn, table)
	}
}

func TestOpenClient_MigratesToVersion1(t *testing.T) {
	store := setupTestClientDB(t)

	if version := schemaVersion(t, store.Conn); version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
	}
}

func TestOpenClient_CreatesPeerTable(t *testing.T) {
	store := setupTestClientDB(t)

	assertTableExists(t, store.Conn, "peer")
}

func TestOpenServer_ReopenIsIdempotent(t *testing.T) {
	opts := database.Options{Name: "reopen-test", Dir: t.TempDir()}

	store, err := database.OpenServer(opts)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := createRootCidr(t, store, TestCidrRoot); err != nil {
		t.Fatalf("failed to create root cidr: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("failed to close database: %v", err)
	}

	store, err = database.OpenServer(opts)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if version := schemaVersion(t, store.Conn); version != 1 {
		t.Fatalf("user_version after reopen = %d, want 1", version)
	}
	assertCidrExists(t, store, TestCidrRoot)
}

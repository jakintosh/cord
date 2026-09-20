package database_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestOpen_UserVersionSet(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	var version int
	if err := db.Conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Fatalf("user_version = %d, want 1", version)
	}
}

func TestOpen_TablesExist(
	t *testing.T,
) {
	db := testutil.SetupDB(t)

	for _, table := range []string{
		"network",
		"group",
		"association",
		"peer",
		"cidr",
		"cidr_assignment",
		"registration",
		"registration_assignment",
		"endpoint",
	} {
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

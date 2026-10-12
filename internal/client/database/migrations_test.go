package database_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestMigrations_InitializeSchema(
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

	for _, table := range []string{"network", "peer", "endpoint", "install", "topology"} {
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
	var index string
	if err := db.Conn.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_peer_endpoint_lookup'`,
	).Scan(&index); err != nil {
		t.Fatalf("endpoint lookup index should exist: %v", err)
	}
}

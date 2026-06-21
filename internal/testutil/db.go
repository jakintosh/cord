package testutil

import (
	"testing"

	clientdatabase "git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
)

func SetupTestDB(t *testing.T) *database.DB {
	t.Helper()

	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func SetupClientTestDB(t *testing.T) *clientdatabase.DB {
	t.Helper()

	opts := clientdatabase.Options{
		Path: ":memory:",
	}
	db, err := clientdatabase.Open(opts)
	if err != nil {
		t.Fatalf("open client db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

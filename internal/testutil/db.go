package testutil

import (
	"testing"

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

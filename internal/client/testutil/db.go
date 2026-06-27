package testutil

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
)

func SetupDB(
	t *testing.T,
) *database.DB {
	t.Helper()

	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open client db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

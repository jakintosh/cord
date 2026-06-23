package testutil

import (
	"log"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func SetupDB(t *testing.T) *database.DB {
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

type Env struct {
	DB      *database.DB
	Service *service.Service
}

func SetupService(t *testing.T, wg service.WG) *Env {
	t.Helper()

	db := SetupDB(t)

	svcOpts := service.Options{
		Store:  db,
		WG:     wg,
		Clock:  func() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) },
		Logger: log.Default(),
	}
	svc, err := service.New(svcOpts)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &Env{
		DB:      db,
		Service: svc,
	}
}

package testutil

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// ServiceEnv is a domain-only environment: a service over an in-memory
// database. It has no runtime or live resources.
type ServiceEnv struct {
	Database *database.DB
	Service  *service.Service
}

func SetupService(
	t *testing.T,
) *ServiceEnv {
	t.Helper()
	return SetupServiceWithClock(t, func() time.Time { return FixedTime })
}

func SetupServiceWithClock(
	t *testing.T,
	clock func() time.Time,
) *ServiceEnv {
	t.Helper()

	db := SetupDB(t)

	svc, err := service.New(service.Options{
		Store:  db,
		Clock:  clock,
		Logger: logging.Discard(),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &ServiceEnv{
		Database: db,
		Service:  svc,
	}
}

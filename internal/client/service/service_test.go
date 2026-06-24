package service_test

import (
	"context"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func TestStart_NoStore(t *testing.T) {
	svc, err := service.New(service.Options{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	err = svc.Start(context.Background())
	if !errors.Is(err, service.ErrNotImplemented) {
		t.Errorf("err = %v, want ErrNotImplemented", err)
	}
}

func TestStart_NoWG(t *testing.T) {
	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	svc, err := service.New(service.Options{
		Store: db,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	err = svc.Start(context.Background())
	if !errors.Is(err, service.ErrWireGuardUnavailable) {
		t.Errorf("err = %v, want ErrWireGuardUnavailable", err)
	}
}

func TestClose_StopsRunningNetworks(t *testing.T) {
	env := setupTestEnv(t)
	seedNetwork(t, env.svc, "close-me")

	ctx := context.Background()
	if err := env.svc.EnableNetwork(ctx, "close-me"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.svc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Device should have been cleaned up
	d, ok := env.wg.Devices["close-me"]
	if !ok {
		t.Fatal("expected device was created")
	}
	if d.DownCalls != 1 {
		t.Errorf("down calls = %d, want 1", d.DownCalls)
	}

	// Status should show not running
	statuses, err := env.svc.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, st := range statuses {
		if st.Running {
			t.Errorf("%s should not be running after close", st.Name)
		}
	}
}

func TestClose_NoRunningNetworks(t *testing.T) {
	env := setupTestEnv(t)

	if err := env.svc.Close(); err != nil {
		t.Fatalf("close with nothing running: %v", err)
	}
}

package service_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestNew_RequiresStore(
	t *testing.T,
) {
	svc, err := service.New(service.Options{})
	if err == nil || svc != nil {
		t.Fatalf("New without store = (%v, %v), want nil service and error", svc, err)
	}
}

func TestNew_DefaultsClockAndLogger(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	svc, err := service.New(service.Options{Store: db})
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now().Truncate(time.Second)
	install, err := svc.BeginInstall(
		testutil.Invitation("testnet", "10.43.0.1:8443"),
		service.NetworkOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if install.CreatedAt.Before(before) || install.CreatedAt.After(time.Now()) {
		t.Fatalf("created at = %v, outside operation time", install.CreatedAt)
	}
}

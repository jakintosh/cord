package service_test

import (
	"context"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestClose_StopsRunningNetworks(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "close-me")

	ctx := context.Background()
	if err := env.Service.EnableNetwork(ctx, "close-me"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.Service.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	dev := env.Backend.Device("close-me")
	if dev == nil {
		t.Fatal("expected device was created")
	}
	if dev.CloseCalls != 1 {
		t.Errorf("close calls = %d, want 1", dev.CloseCalls)
	}

	statuses, err := env.Service.Status()
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
	env := testutil.SetupService(t)

	if err := env.Service.Close(); err != nil {
		t.Fatalf("close with nothing running: %v", err)
	}
}

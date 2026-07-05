package service_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestClose_StopsRunningNetworks(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "close-me")

	if err := env.Service.EnableNetwork("close-me"); err != nil {
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

	if env.Service.IsNetworkRunning("close-me") {
		t.Error("network should not be running after close")
	}
}

func TestClose_NoRunningNetworks(t *testing.T) {
	env := testutil.SetupService(t)

	if err := env.Service.Close(); err != nil {
		t.Fatalf("close with nothing running: %v", err)
	}
}

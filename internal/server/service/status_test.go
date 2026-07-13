package service_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestStatusReportsEnabledAndRunningState(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	status, err := env.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Networks) != 1 {
		t.Fatalf("networks = %d, want 1", len(status.Networks))
	}
	if status.Networks[0].Enabled || status.Networks[0].Running {
		t.Fatalf("initial status = %+v, want disabled and stopped", status.Networks[0])
	}

	if err := env.Service.EnableNetwork("testnet"); err != nil {
		t.Fatalf("enable network: %v", err)
	}

	status, err = env.Service.Status()
	if err != nil {
		t.Fatalf("status after enable: %v", err)
	}
	if !status.Networks[0].Enabled || !status.Networks[0].Running {
		t.Fatalf("enabled status = %+v, want enabled and running", status.Networks[0])
	}
}

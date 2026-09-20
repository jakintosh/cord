package runtime_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestGetOverview_ReportsDeviceFailureWithoutRepairing(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	device := env.Backend.Device("testnet")
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		status := networkStatus(t, env.Runtime, "testnet")
		if !status.Running || status.Health != runtime.HealthDegraded || status.Status.Main.WireGuard.State != resource.Failed {
			t.Fatalf("device failure status = %+v", status)
		}
		if status.Status.Reconcile.Error != "" || env.Backend.Device("testnet") != device {
			t.Fatal("status read ran maintenance")
		}
	}
	env.ReconcileNetwork(t, "testnet")
	if status := networkStatus(t, env.Runtime, "testnet"); status.Health != runtime.HealthHealthy {
		t.Fatalf("recovery status = %+v", status)
	}
}

func networkStatus(
	t *testing.T,
	rt *runtime.Runtime,
	name string,
) runtime.NetworkOverview {
	t.Helper()

	status, err := rt.GetOverview(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, network := range status.Networks {
		if network.Name == name {
			return network
		}
	}
	t.Fatalf("network %q missing from status", name)
	return runtime.NetworkOverview{}
}

package runtime_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
)

func TestGetStatus_ObservesLostDeviceWithoutRepair(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "office")
	env.Enable(t, "office")
	device := env.Backend.Device("office")
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}
	creates := len(env.Backend.CreateCalls)
	closes := device.CloseCalls

	status, err := env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Snapshot.Children) != 1 {
		t.Fatalf("resource tree = %+v", status.Snapshot)
	}
	network := status.Snapshot.Children[0]
	tunnel := network.Children[0]
	leaf := tunnel.Children[0]
	if network.Kind != "network" || tunnel.Kind != "tunnel" || leaf.Kind != "wireguard-device" {
		t.Fatalf("unexpected tree: %+v", network)
	}
	if status.Health != runtime.HealthDegraded || network.Health != runtime.HealthDegraded ||
		tunnel.Health != runtime.HealthDegraded || leaf.Health != runtime.HealthDegraded {
		t.Fatal("device failure did not propagate through ancestor health")
	}
	if leaf.Error == "" || leaf.Target != resource.Running {
		t.Fatalf("device failure = %+v", leaf)
	}
	if len(env.Backend.CreateCalls) != creates || device.CloseCalls != closes {
		t.Fatal("status performed resource repair or cleanup")
	}

	// Returned maps and slices belong to the caller.
	status.Snapshot.Children[0].Activities["sync"] = runtime.ActivityStatus{Error: "caller mutation"}
	status.Snapshot.Children[0].Children[0].Kind = "changed"
	again, err := env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if again.Snapshot.Children[0].Activities["sync"].Error == "caller mutation" ||
		again.Snapshot.Children[0].Children[0].Kind != "tunnel" {
		t.Fatal("snapshot aliases live resource state")
	}
}

func TestGetStatus_RetainsUninstalledNetworkUntilCleanup(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "office")
	env.Enable(t, "office")
	device := env.Backend.Device("office")
	device.CloseErr = errors.New("cleanup blocked")
	t.Cleanup(func() { device.CloseErr = nil })
	if err := env.Runtime.UninstallNetwork(t.Context(), "office"); err != nil {
		t.Fatal(err)
	}
	status, err := env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Networks) != 0 || len(status.Snapshot.Children) != 1 {
		t.Fatalf("deleted record hid owned resource: %+v", status)
	}
	network := status.Snapshot.Children[0]
	if network.Target != resource.Stopped || network.Activities["close"].Error == "" ||
		status.Health != runtime.HealthDegraded {
		t.Fatalf("cleanup status = %+v", network)
	}
	device.CloseErr = nil
	if err := env.Runtime.ConvergeAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err = env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Snapshot.Children) != 0 || status.Health != runtime.HealthHealthy {
		t.Fatalf("successful cleanup remained unhealthy: %+v", status)
	}
}

func TestGetStatus_ReportsMaintenanceAndStorageFailure(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetworkDirect(t, env.Database, "office")
	env.Enable(t, "office")
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := env.Runtime.ConvergeAll(t.Context()); err == nil {
		t.Fatal("maintenance unexpectedly succeeded")
	}
	status, err := env.Runtime.GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Snapshot.Error == "" || status.Snapshot.Activities["reconcile"].Error == "" ||
		len(status.Snapshot.Children) != 1 || status.Health != runtime.HealthDegraded {
		t.Fatalf("storage failure hid resource diagnostics: %+v", status)
	}
}

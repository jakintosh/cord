package runtime_test

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestGetStatus_ReportsDeviceFailureWithoutRepairing(
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
		if status.Snapshot == nil || status.Health != resource.HealthDegraded || resourceAt(t, *status.Snapshot, "main", "wireguard-device").State != resource.Failed {
			t.Fatalf("device failure status = %+v", status)
		}
		if status.Reconcile.Error != "" || env.Backend.Device("testnet") != device {
			t.Fatal("status read ran maintenance")
		}
	}
	env.ReconcileNetwork(t, "testnet")
	if status := networkStatus(t, env.Runtime, "testnet"); status.Health != resource.HealthHealthy {
		t.Fatalf("recovery status = %+v", status)
	}
}

func TestGetStatus_RetainsDeletedNetworkUntilCleanupSucceeds(
	t *testing.T,
) {
	synctest.Test(t, func(t *testing.T) {
		env := testutil.SetupRuntime(t)
		testutil.SeedNetwork(t, env.Service)
		env.Enable(t, "testnet")
		device := env.Backend.Device("testnet")
		device.CloseErr = errors.New("device close failed")
		t.Cleanup(func() { device.CloseErr = nil })
		if _, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", false); err != nil {
			t.Fatal(err)
		}
		if err := env.Runtime.DeleteNetwork(t.Context(), "testnet"); err != nil {
			t.Fatal(err)
		}

		status := testutil.RuntimeStatus(t, env.Runtime)
		if len(status.Networks) != 0 || status.Health != resource.HealthDegraded {
			t.Fatalf("deleted network status = %+v", status)
		}
		network := resourceAt(t, status.Snapshot, "testnet")
		if network.Target != resource.Stopped || network.State != resource.Failed || network.Activities["close"].Error == "" {
			t.Fatalf("cleanup status = %+v", network)
		}
		leaf := resourceAt(t, network, "main", "wireguard-device")
		if leaf.Target != resource.Stopped || leaf.Health != resource.HealthDegraded || leaf.Error == "" {
			t.Fatalf("retained device = %+v", leaf)
		}

		device.CloseErr = nil
		// Synchronize the fixture change with the runtime before its timer fires.
		_ = testutil.RuntimeStatus(t, env.Runtime)
		time.Sleep(runtime.DefaultInterval + time.Second)
		synctest.Wait()
		status = testutil.RuntimeStatus(t, env.Runtime)
		if len(status.Snapshot.Children) != 0 || status.Health != resource.HealthHealthy {
			t.Fatalf("completed cleanup = %+v", status)
		}
	})
}

func TestGetStatus_ReturnsDetachedTree(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	env.Enable(t, "testnet")
	status := testutil.RuntimeStatus(t, env.Runtime)
	status.Snapshot.Children[0].Name = "changed"
	status.Snapshot.Children[0].Children[0].Children[0].State = resource.Failed
	delete(status.Snapshot.Children[0].Activities, "reconcile")
	delete(status.Snapshot.Activities, "reconcile")

	status = testutil.RuntimeStatus(t, env.Runtime)
	network := resourceAt(t, status.Snapshot, "testnet")
	if network.Activities["reconcile"].LastSuccessAt.IsZero() || status.Snapshot.Activities["reconcile"].LastSuccessAt.IsZero() {
		t.Fatal("snapshot mutation changed recorded activity")
	}
	if leaf := resourceAt(t, network, "main", "wireguard-device"); leaf.State != resource.Running {
		t.Fatalf("snapshot mutation changed resource state: %+v", leaf)
	}
}

type networkObservation struct {
	runtime.NetworkStatus
	Snapshot *resource.Snapshot
}

func networkStatus(
	t *testing.T,
	rt *runtime.Runtime,
	name string,
) networkObservation {
	t.Helper()

	status, err := rt.GetStatus(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, network := range status.Networks {
		if network.Name == name {
			observation := networkObservation{NetworkStatus: network}
			for i := range status.Snapshot.Children {
				child := &status.Snapshot.Children[i]
				if child.Name == name {
					observation.Snapshot = child
					break
				}
			}
			return observation
		}
	}
	t.Fatalf("network %q missing from status", name)
	return networkObservation{}
}

// resourceAt follows names (for owners) or kinds (for unnamed leaves).
func resourceAt(
	t *testing.T,
	snapshot resource.Snapshot,
	path ...string,
) resource.Snapshot {
	t.Helper()
	for _, name := range path {
		found := false
		for _, child := range snapshot.Children {
			if child.Name == name || child.Name == "" && child.Kind == name {
				snapshot = child
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("resource %q missing from %+v", name, snapshot)
		}
	}
	return snapshot
}
